# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go Notify is a **library, not a CLI** — it has no `main`, no `cmd/`, and no cobra dependency. It is the extraction of `go-notifier/pkg/slack` plus a new channel-agnostic layer on top. The sibling repo `go-notifier` (Slack + Backlog + CLI) is being retired in its favour; nothing here should grow a command-line surface.

Six services consume it (`ap-chain`, `ap-comic`, `ap-comp`, `ap-mv`, `ap-voice`, `git-gemini-web`), each of which previously hand-rolled its own `internal/adapters/slack.go`. The `notify` package exists to absorb what those six duplicated.

## Commands

```bash
go build ./...
go vet ./...
gofmt -l .                        # must print nothing; `gofmt -w .` to fix
go test -race ./...               # full suite
go test -race ./slack/... -run TestName   # single test
golangci-lint run                 # v2.12.2, config in .golangci.yml
govulncheck ./...
```

CI (`.github/workflows/ci.yml`) runs three jobs on push/PR to `main`/`develop`: build+vet+gofmt+race-tests, golangci-lint, govulncheck. Branches: work on `develop`, `main` is the release branch, tags drive pkg.go.dev.

## Architecture

```
notify.go     # Notifier / Message / Disabled — the abstraction
body.go       # Body — notification body builder
pipeline.go   # Pipeline — success / failure / skipped triple
slack/        # Slack Incoming Webhook implementation
```

`slack` imports `notify`; never the reverse. A new channel is a new subpackage implementing `notify.Notifier` — `notify` itself must not need editing to accommodate one.

### The channel-agnostic boundary

**`notify.Body` emits standard Markdown** (`**bold**`, `[text](url)`), *not* Slack mrkdwn. Converting to a channel's dialect is the channel package's job (`slack/blocks.go` `formatMarkdown`). This is the single most important invariant here: the moment `Body` emits `*bold*` or `<url|text>`, adding Discord means either rewriting every call site or parsing mrkdwn back out. `TestNotifyRendersBodyAsMrkdwn` in `slack/notifier_test.go` pins the seam.

`Body` methods skip empty values, which is what removes the per-field `if value != ""` blocks the six services all carried. `String()` on a Body that was never written to returns `NotAvailable` (`"N/A"`) rather than an empty string — an empty notification is never the intent, and Slack's section block rejects empty text.

### Disabled, not error

`slack.NewNotifier` returns `notify.Disabled()` when the webhook URL is blank or whitespace, and only errors when a URL *is* configured but the HTTP client is nil. This unifies a behaviour that had drifted: five services treated a blank webhook as "notifications off", while `ap-voice` let `slack.NewClient` fail and turned a missing optional setting into a startup error.

`notify.Enabled(n)` reports whether a notifier actually sends, so callers can skip expensive body construction. `Pipeline.Enabled()` forwards it.

Note the two constructors differ on purpose: `slack.NewClient` still rejects an empty webhook URL (it returns a concrete `*Client`, and a client that cannot post is meaningless). `NewNotifier` is the one with the optional-feature semantics.

### Slack escaping rules

`formatMarkdown` converts links **before** escaping, and this order is load-bearing. Slack requires `&`, `<`, `>` to be HTML-escaped **in plain message text only** — inside `<URL|text>` constructs and mentions they are already parsed as syntax. Escaping a GCS signed URL's `&` separators would rewrite the signature and produce a 403, so `preservedRegex` deliberately skips scheme-prefixed links (`<https://…>`, `<mailto:…>`) and mentions (`<@U…>`, `<#C…>`, `<!here>`).

`preservedRegex` matches only *those* constructs, not any `<…>`, because an error string containing `<nil>` must be escaped rather than mistaken for a link. That distinction is what `TestFormatMarkdownEscapesSpecialCharacters` and `TestFormatMarkdownSignedURLIsNotEscaped` guard from opposite sides — changing one without the other will break the other's case.

Blockquote (`>` at line start) is consequently unsupported; it is indistinguishable from a character needing escape.

### HTTP

No package here owns HTTP concerns. All outbound traffic goes through `go-http-kit`'s `httpkit.Requester`, injected into constructors — it carries timeouts, exponential-backoff retry (`netarmor/retry`), SSRF-safe dialing, and response-size limits. Take the narrow `httpkit.Requester` interface, not `*httpkit.Client`, so tests can substitute a fake. Package code contains no retry loops, no `http.Client`, no status-code handling.

### Slack package internals

- `client.go` — `Client` holds the requester, webhook URL, and display overrides. `SendTextWithHeader` posts an explicit header + body; `SendText` derives the header from the body's first line (`📢 ` prefix, 50 runes, `📢 通知メッセージ` fallback).
- `notifier.go` — `Notify` implements `notify.Notifier`, falling back to `SendText` when `Message.Title` is empty. `NewNotifier` holds the disabled-vs-error policy above.
- `options.go` — `WithUsername`, `WithIconEmoji`, `WithChannel`.
- `blocks.go` — Markdown→mrkdwn conversion, escaping, Block Kit assembly, and section truncation at `maxSectionLength` (2900 runes, under Slack's 3000 limit).

Truncation is rune-based (`utf8.RuneCountInString` / `go-utils/text.Truncate`), never byte-based — messages are Japanese.

Config comes from the environment at the call site, never inside the package: `SLACK_WEBHOOK_URL`, `SLACK_USERNAME`, `SLACK_ICON_EMOJI`, `SLACK_CHANNEL`.

## Conventions

- Doc comments are Japanese, one per exported *and* unexported symbol, in the `名前 は …します。` form. Error strings are Japanese; wrap with `%w`.
- Constructors return `(*T, error)` and validate up front; configuration via `type Option func(*T)` variadics.
- Tests are black-box (`package notify_test` / `package slack_test`) driving a stub `httpkit.Requester`. Use white-box `_internal_test.go` only for unexported helpers — `slack/blocks_internal_test.go` is the one such file, because the Markdown conversion rules are worth testing directly rather than through a webhook payload.
- Comments explain *why*, not *what*. The escaping-order and preserved-construct comments in `blocks.go` are the model: each states the failure it prevents.
