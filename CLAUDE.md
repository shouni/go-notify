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
notify/       # the channel-agnostic abstraction
  notify.go     # Notifier / Message / Level / Disabled
  body.go       # Body — notification body builder
  pipeline.go   # Pipeline — success / failure / skipped triple, WithTitles for per-call headings
slack/        # Slack Incoming Webhook implementation
```

Both packages live in subdirectories; nothing sits at the module root. That matches every other library in this family (`go-http-kit/httpkit`, `go-remote-io/remoteio`, `go-utils/*`) — `clibase` is the only sibling with a root package. The import path therefore repeats the name (`github.com/shouni/go-notify/notify`), exactly as `go-remote-io/remoteio` does.

`slack` imports `notify`; never the reverse. A new channel is a new subpackage implementing `notify.Notifier` — `notify` itself must not need editing to accommodate one.

### The channel-agnostic boundary

**`notify.Body` emits standard Markdown** (`**bold**`, `[text](url)`), *not* Slack mrkdwn. Converting to a channel's dialect is the channel package's job (`slack/blocks.go` `formatMarkdown`). This is the single most important invariant here: the moment `Body` emits `*bold*` or `<url|text>`, adding Discord means either rewriting every call site or parsing mrkdwn back out. `TestNotifyRendersBodyAsMrkdwn` in `slack/notifier_test.go` pins the seam.

`Body` methods skip empty values, which is what removes the per-field `if value != ""` blocks the six services all carried. `String()` on a Body that was never written to returns `NotAvailable` (`"N/A"`) rather than an empty string — an empty notification is never the intent, and Slack's section block rejects empty text.

`notify.CodeSpan` is the deliberate escape hatch from that seam. `Code` only builds "label + one value", so callers wanting a unit, an emoji, or two monospaced values on one line were writing backticks inline (`fmt.Sprintf("`%d` 🎲", seed)` in `ap-comp`, `fmt.Sprintf("`%s` ← `%s`", base, feature)` in `git-gemini-web`) — which is the invariant above starting to leak into consumers. `CodeSpan` gives them the markup without letting them author it. If a new case can't be expressed with `CodeSpan` + `Field`, add a `Body` method rather than letting a call site write Markdown.

`Body.Block` emits its fences on their own lines. `` ```content``` `` on one line is not a fenced block in CommonMark — the first content line is eaten as the info string and the closing fence never matches — and `slack/blocks.go` relies on the line-anchored form to find blocks to protect.

### Disabled, not error

`slack.NewNotifier` returns `notify.Disabled()` when the webhook URL is blank or whitespace, and only errors when a URL *is* configured but the HTTP client is nil. This unifies a behaviour that had drifted: five services treated a blank webhook as "notifications off", while `ap-voice` turned a missing optional setting into a startup error.

`notify.Enabled(n)` reports whether a notifier actually sends, so callers can skip expensive body construction. `Pipeline.Enabled()` forwards it.

`Pipeline.WithTitles` returns a copy with different headings and exists because `ap-comic` needs a per-command title and, without it, dropped out of `Pipeline` entirely — re-declaring `errorLabel`, calling `notify.Enabled` by hand, and open-coding `Notify` plus its error wrapping in two places. Titles are replaced, not merged: each call site fills in only the outcome it is about to send.

### Level

`Message.Level` carries the outcome in a form a channel can act on, which a heading string cannot. All six services had been encoding it as `✅` / `❌` / `⏭️` inside their title text — six independent instances of the same workaround, which is what justified adding it.

`Pipeline` sets it; callers never pass it. Letting a caller supply both a heading and a level only creates room for the two to disagree. `LevelNone` is the zero value, so a `Message` built by hand keeps its current behaviour, and `slack` renders it exactly as before.

Rendering is the channel's call. `slack` maps the three levels to `good` / `danger` / `warning` and wraps the blocks in a coloured attachment; `LevelNone` stays as top-level blocks, because an attachment indents the body and there is no reason to change how existing, level-less notifications look.

The two branches set the fallback text differently, and this is not cosmetic. With top-level `blocks`, Slack treats `WebhookMessage.Text` as fallback only and does not render it. Move the blocks into an attachment and `Text` becomes actual message body, so leaving the title there prints it twice — once as text, once in the attachment's header block. The coloured branch therefore leaves `Text` empty and puts the title in `Attachment.Fallback`, which keeps push-notification wording intact. `TestNotifyLevelDoesNotDuplicateTitle` pins it; v1.2.0 and v1.2.1 shipped without it and duplicated every heading.

`NewNotifier` is the only exported constructor. A `NewClient` returning a concrete `*Client`, plus `SendText` / `SendTextWithHeader`, used to sit beneath it; all six consumers went through `NewNotifier` and none touched them, so they were removed rather than kept as a second way in.

### Slack escaping rules

`formatMarkdown` converts links **before** escaping, and this order is load-bearing. Slack requires `&`, `<`, `>` to be HTML-escaped **in plain message text only** — inside `<URL|text>` constructs and mentions they are already parsed as syntax. Escaping a GCS signed URL's `&` separators would rewrite the signature and produce a 403, so `preservedRegex` deliberately skips scheme-prefixed links (`<https://…>`, `<mailto:…>`) and mentions (`<@U…>`, `<#C…>`, `<!here>`).

`preservedRegex` matches only *those* constructs, not any `<…>`, because an error string containing `<nil>` must be escaped rather than mistaken for a link. That distinction is what `TestFormatMarkdownEscapesSpecialCharacters` and `TestFormatMarkdownSignedURLIsNotEscaped` guard from opposite sides — changing one without the other will break the other's case.

Blockquote (`>` at line start) is consequently unsupported; it is indistinguishable from a character needing escape.

Fenced code blocks are exempt from markup conversion but not from escaping. `formatMarkdown` splits on `fencedBlockRegex` and runs `convertMarkdown` only outside the fences; inside, it applies `mrkdwnEscaper` directly rather than `escapeMrkdwn`, because a block exists to show text verbatim — a string that happens to look like `<https://…>` is a string there, not a link. Without the split, `Body.Block` mangles exactly what it is for: a pasted stack trace's `- ` becomes `• ` and `**` becomes `*`. `TestFormatMarkdownPreservesCodeBlockContent` and `TestFormatMarkdownConvertsAroundCodeBlock` pin the two sides. An unterminated fence deliberately does *not* match, so one broken fence can't silently exempt the rest of the message.

`listItemRegex` uses `[ \t]` rather than `\s` for the same reason `preservedRegex` is narrow: `\s` swallows the preceding newline, so a list item at the start of a segment consumed the line break separating it from the previous line. That only surfaced once `formatMarkdown` began converting segments instead of the whole message.

### HTTP

No package here owns HTTP concerns. All outbound traffic goes through `go-http-kit`'s `httpkit.Requester`, injected into constructors — it carries timeouts, exponential-backoff retry (`netarmor/retry`), SSRF-safe dialing, and response-size limits. Take the narrow `httpkit.Requester` interface, not `*httpkit.Client`, so tests can substitute a fake. Package code contains no retry loops, no `http.Client`, no status-code handling.

That includes the retry policy, and the consequence is documented rather than fixed here: **webhook posts are not idempotent** — each successful one creates a new Slack message, so a retry after a lost response posts the notification twice. `httpkit.New(timeout)` enables retry by default, so a caller sharing one client across all its outbound traffic gets that behaviour for notifications too. The fix belongs to the caller — `slack.NewNotifier(httpClient.WithoutRetry(), url)`, which derives a no-retry client sharing the original's timeout, SSRF settings, and connection pool — because whether a duplicate notification is worse than a dropped one is an application decision. Do not paper over it by constructing a client inside this package — that would silently discard the injected one and take the choice away.

The sibling repo `ap-mcp-slack` reached the opposite default for the same reason and disables retry explicitly in `internal/client/webhook.go`; it deliberately does *not* use this library, since its callers supply raw mrkdwn and Block Kit that must not be reformatted or escaped.

### Slack package internals

- `notifier.go` — the unexported `notifier` holds the requester, webhook URL, and display overrides. `NewNotifier` holds the disabled-vs-error policy above; `Notify` implements `notify.Notifier`. An empty `Message.Title` is an error, not a cue to guess: deriving a heading from the body's first line was a feature nothing used, and `Pipeline.send` already rejects an empty title, so the library said no through two separate paths.
- `options.go` — `WithUsername`, `WithIconEmoji`, `WithChannel`.
- `blocks.go` — Markdown→mrkdwn conversion, escaping, Block Kit assembly, and section truncation at `maxSectionLength` (2900 runes, under Slack's 3000 limit).

Truncation is rune-based (`utf8.RuneCountInString` / `go-utils/text.Truncate`), never byte-based — messages are Japanese.

Config comes from the environment at the call site, never inside the package: `SLACK_WEBHOOK_URL`, `SLACK_USERNAME`, `SLACK_ICON_EMOJI`, `SLACK_CHANNEL`.

## Conventions

- Doc comments are Japanese, one per exported *and* unexported symbol, in the `名前 は …します。` form. Error strings are Japanese; wrap with `%w`.
- Constructors return `(*T, error)` and validate up front; configuration via `type Option func(*T)` variadics.
- Tests are black-box (`package notify_test` / `package slack_test`) driving a stub `httpkit.Requester`. Use white-box `_internal_test.go` only for unexported helpers — `slack/blocks_internal_test.go` is the one such file, because the Markdown conversion rules are worth testing directly rather than through a webhook payload.
- Comments explain *why*, not *what*. The escaping-order and preserved-construct comments in `blocks.go` are the model: each states the failure it prevents.
