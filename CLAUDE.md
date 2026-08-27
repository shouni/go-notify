# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go Notify is a **library, not a CLI** — it has no `main`, no `cmd/`, and no cobra dependency. It is the extraction of the retired sibling repo `go-notifier`'s `pkg/slack` (Slack + Backlog + CLI, now deleted) plus a new channel-agnostic layer on top; nothing here should grow a command-line surface.

Several services consume it today, each of which previously hand-rolled its own
`internal/adapters/slack.go`. The `notify` package exists to absorb what those services duplicate. **Do
not list them here** — the list that used to live in this sentence had already gone stale, naming three
repos that no longer exist, and some of the current ones are private:

    grep -l "shouni/go-notify" ~/GolandProjects/*/go.mod

## Commands

```bash
go build ./...
go vet ./...
gofmt -l .                        # must print nothing; `gofmt -w .` to fix
go test -race ./...               # full suite
go test -race ./slack/... -run TestName   # single test
golangci-lint run                 # v2.13.1, config in .golangci.yml
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
docs/slack.md # Slack-specific reference; README stays channel-agnostic
```

Both packages live in subdirectories; nothing sits at the module root. That matches every other library in this family (`go-http-kit/httpkit`, `go-remote-io/remoteio`, `go-utils/*`) — `clibase` is the only sibling with a root package. The import path therefore repeats the name (`github.com/shouni/go-notify/notify`), exactly as `go-remote-io/remoteio` does.

`slack` imports `notify`; never the reverse. A new channel is a new subpackage implementing `notify.Notifier` — `notify` itself must not need editing to accommodate one.

### The channel-agnostic boundary

**`notify.Body` emits standard Markdown** (`**bold**`, `[text](url)`), *not* Slack mrkdwn. Converting to a channel's dialect is the channel package's job (`slack/blocks.go` `formatMarkdown`). This is the single most important invariant here: the moment `Body` emits `*bold*` or `<url|text>`, adding Discord means either rewriting every call site or parsing mrkdwn back out. `TestNotifyRendersBodyAsMrkdwn` in `slack/notifier_test.go` pins the seam.

`Body` methods skip empty values, which is what removes the per-field `if value != ""` blocks the six services all carried. `String()` on a Body that was never written to returns `NotAvailable` (`"N/A"`) rather than an empty string — an empty notification is never the intent, and Slack's section block rejects empty text.

`notify.CodeSpan` is the deliberate escape hatch from that seam. `Code` only builds "label + one value", so callers wanting a unit, an emoji, or two monospaced values on one line were writing backticks inline (`fmt.Sprintf("`%d` 🎲", seed)` in `ap-comp`, `fmt.Sprintf("`%s` ← `%s`", base, feature)` in `adk-review`) — which is the invariant above starting to leak into consumers. `CodeSpan` gives them the markup without letting them author it. If a new case can't be expressed with `CodeSpan` + `Field`, add a `Body` method rather than letting a call site write Markdown.

`Body.URIField` absorbs the `writeURIField` + `gcsConsoleURL` pair that `ap-voice` and `ap-mv` carried as identical ~30-line copies (ap-voice's comment even said "ap-mv の writeURIField と同じ形です"): a `gs://` URI renders as a link to Cloud Console with the `gs://` string as display text — the URI stays copy-pasteable into `gcloud storage`, and `gs://` is a dead string in every channel anyway — while any other value falls through to `Field`. GCS-awareness in a channel-agnostic package is deliberate: the boundary here is about output *markup* (Markdown vs mrkdwn), not content sources, and this fleet stores artifacts in GCS only.

`notify.JoinURL` is there for a different flavour of the same duplication. Four of the five consumers carried six near-identical private functions — `ap-comp.detailPageURL`, `ap-mv.draftsURL`, `ap-mv.historyDetailURL`, `ap-story.resultPageURL` (twice), `ap-voice.detailURL` — all shaped "if the base URL or the ID is empty return `""`, join, and return `""` if the join fails". They converge on that shape because `Body.Link` skips a row whose URL is empty, and that contract belongs to this package; the URL assembly that satisfies it therefore belongs here too. It returns a string rather than `(string, error)` for the same reason: an error would just be turned back into `""` by every caller.

`Body.Heading` and `Body.Bullet` exist for the same reason. `slack/blocks.go` converts `## ` and `- ` (`headerRegex`, `listItemRegex`), but for a while no `Body` method emitted either, so a caller wanting a sub-heading or a variable-length list had to hand-write the markup through `Text` — the same leak `CodeSpan` was added to plug. `TestNotifyRendersHeadingAndBulletAsMrkdwn` pins the pair to the conversion.

`Body.Block` emits its fences on their own lines. `` ```content``` `` on one line is not a fenced block in CommonMark — the first content line is eaten as the info string and the closing fence never matches — and `slack/blocks.go` relies on the line-anchored form to find blocks to protect.

### Disabled, not error

`slack.NewNotifier` returns `notify.Disabled()` when the webhook URL is blank or whitespace, and only errors when a URL *is* configured but the HTTP client is nil. This unifies a behaviour that had drifted: five services treated a blank webhook as "notifications off", while `ap-voice` turned a missing optional setting into a startup error.

`notify.Enabled(n)` reports whether a notifier actually sends, so callers can skip expensive body construction. `Pipeline.Enabled()` forwards it.

`Pipeline.Failure` and `Pipeline.Skipped` append their trailing section to a *copy* of the caller's `Body` (`pipeline.go: derive`, `body.go: clone`). They used to write straight into the caller's `Body`, and a test pinned that as intended — but the API hands the same `*Body` back for the next outcome, so reuse is the shape it invites, and reusing one produced a skipped notification carrying the previous failure's "エラー内容". The README's own `Success` → `Failure` → `Skipped` example demonstrated it. `clone` copies the raw builder, not `String()`: `String()` trims the trailing newline, and `separate()` reads that newline to decide whether to insert a blank line, so a round-trip through `String()` silently loses the blank line before the appended section. `TestPipelineBodyReuseAcrossOutcomes` pins all three outcomes.

`Pipeline.WithTitles` returns a copy with different headings and exists because `ap-comic` needs a per-command title and, without it, dropped out of `Pipeline` entirely — re-declaring `errorLabel`, calling `notify.Enabled` by hand, and open-coding `Notify` plus its error wrapping in two places. Titles are replaced, not merged: each call site fills in only the outcome it is about to send.

### Level

`Message.Level` carries the outcome in a form a channel can act on, which a heading string cannot. All six services had been encoding it as `✅` / `❌` / `⏭️` inside their title text — six independent instances of the same workaround, which is what justified adding it.

`Pipeline` sets it; callers never pass it. Letting a caller supply both a heading and a level only creates room for the two to disagree. `LevelNone` is the zero value, so a `Message` built by hand keeps its current behaviour, and `slack` renders it exactly as before.

Both places that enumerate `Level` are exhaustive on purpose and the `exhaustive` linter (`check: [switch, map]`) holds them that way. `Level.String()` lists `LevelNone` explicitly instead of folding it into `default`, and `slack.levelColors` carries `LevelNone: ""` instead of omitting the key. The map is the reason the linter is configured for maps at all: a missing key returns the zero value, so a new level added without a colour would post silently uncoloured rather than fail to compile. Adding a level now fails lint in both packages until it is handled.

Rendering is the channel's call. `slack` maps the three levels to `good` / `danger` / `warning` and wraps the blocks in a coloured attachment; `LevelNone` stays as top-level blocks, because an attachment indents the body and there is no reason to change how existing, level-less notifications look.

The two branches set the fallback text differently, and this is not cosmetic. With top-level `blocks`, Slack treats `WebhookMessage.Text` as fallback only and does not render it. Move the blocks into an attachment and `Text` becomes actual message body, so leaving the title there prints it twice — once as text, once in the attachment's header block. The coloured branch therefore leaves `Text` empty and puts the title in `Attachment.Fallback`, which keeps push-notification wording intact. `TestNotifyLevelDoesNotDuplicateTitle` pins it; v1.2.0 and v1.2.1 shipped without it and duplicated every heading.

`NewNotifier` is the only exported constructor. A `NewClient` returning a concrete `*Client`, plus `SendText` / `SendTextWithHeader`, used to sit beneath it; all six consumers went through `NewNotifier` and none touched them, so they were removed rather than kept as a second way in.

### Slack escaping rules

`formatMarkdown` and `escapeMrkdwn` are both one call to `replaceSegments`, which walks a regex's matches and applies one transform inside the matched spans and another outside. The two differ only in the regex and the pair of transforms; the traversal was duplicated verbatim before.

`formatMarkdown` converts links **before** escaping, and this order is load-bearing. Slack requires `&`, `<`, `>` to be HTML-escaped **in plain message text only** — inside `<URL|text>` constructs and mentions they are already parsed as syntax. Escaping a GCS signed URL's `&` separators would rewrite the signature and produce a 403, so `preservedRegex` deliberately skips scheme-prefixed links (`<https://…>`, `<mailto:…>`) and mentions (`<@U…>`, `<#C…>`, `<!here>`).

`preservedRegex` matches only *those* constructs, not any `<…>`, because an error string containing `<nil>` must be escaped rather than mistaken for a link. That distinction is what `TestFormatMarkdownEscapesSpecialCharacters` and `TestFormatMarkdownSignedURLIsNotEscaped` guard from opposite sides — changing one without the other will break the other's case.

Blockquote (`>` at line start) is consequently unsupported; it is indistinguishable from a character needing escape.

Fenced code blocks are exempt from markup conversion but not from escaping. `formatMarkdown` splits on `fencedBlockRegex` and runs `convertMarkdown` only outside the fences; inside, it applies `mrkdwnEscaper` directly rather than `escapeMrkdwn`, because a block exists to show text verbatim — a string that happens to look like `<https://…>` is a string there, not a link. Without the split, `Body.Block` mangles exactly what it is for: a pasted stack trace's `- ` becomes `• ` and `**` becomes `*`. `TestFormatMarkdownPreservesCodeBlockContent` and `TestFormatMarkdownConvertsAroundCodeBlock` pin the two sides. An unterminated fence deliberately does *not* match, so one broken fence can't silently exempt the rest of the message.

`linkRegex` allows one level of balanced parentheses in the URL. Ending the URL at the first `)` truncated links like `https://example.com/a_(b)_c` mid-path and spliced the remainder in after the display text, producing `<https://example.com/a_(b|text>_c)` — worse than not converting at all. A URL whose parens don't balance still doesn't match, and that is the intended fallback: literal Markdown is readable, a mangled link is not.

`listItemRegex` uses `[ \t]` rather than `\s` for the same reason `preservedRegex` is narrow: `\s` swallows the preceding newline, so a list item at the start of a segment consumed the line break separating it from the previous line. That only surfaced once `formatMarkdown` began converting segments instead of the whole message.

### HTTP

No package here owns HTTP concerns. All outbound traffic goes through `go-http-kit`'s `httpkit.Requester`, injected into constructors — it carries timeouts, exponential-backoff retry (`netarmor/retry`), SSRF-safe dialing, and response-size limits. Take the narrow `httpkit.Requester` interface, not `*httpkit.Client`, so tests can substitute a fake. Package code contains no retry loops, no `http.Client`, no status-code handling.

That includes the retry policy, and the consequence is documented rather than fixed here: **webhook posts are not idempotent** — each successful one creates a new Slack message, so a retry after a lost response posts the notification twice. `httpkit.New(timeout)` enables retry by default, so a caller sharing one client across all its outbound traffic gets that behaviour for notifications too. The fix belongs to the caller — `slack.NewNotifier(httpClient.WithoutRetry(), url)`, which derives a no-retry client sharing the original's timeout, SSRF settings, and connection pool — because whether a duplicate notification is worse than a dropped one is an application decision. Do not paper over it by constructing a client inside this package — that would silently discard the injected one and take the choice away.

The sibling repo `ap-mcp-slack` reached the opposite default for the same reason and disables retry explicitly in `internal/client/webhook.go`; it deliberately does *not* use this library, since its callers supply raw mrkdwn and Block Kit that must not be reformatted or escaped.

### Slack package internals

- `notifier.go` — the unexported `notifier` holds the requester and the webhook URL, nothing else. `NewNotifier` holds the disabled-vs-error policy above; `Notify` implements `notify.Notifier`. An empty `Message.Title` is an error, not a cue to guess: deriving a heading from the body's first line was a feature nothing used, and `Pipeline.send` already rejects an empty title, so the library said no through two separate paths.
- `blocks.go` — Markdown→mrkdwn conversion, escaping, Block Kit assembly, and truncation of both blocks: the body at `maxSectionLength` (2900, under Slack's 3000 limit) and the heading at `maxHeaderLength` (150, which *is* Slack's limit). The heading limit is not cosmetic — exceeding it returns `invalid_blocks` and loses the whole notification, so every release before this one could drop a message outright for a long title. `TestNotifyTruncatesLongTitle` pins it.

Truncation measures in runes (`utf8.RuneCountInString`) but cuts in grapheme clusters (`truncate.go: truncateGraphemes`), never in bytes. The cut has to be cluster-based or a combining mark or ZWJ emoji gets split; the guard can stay rune-based because cluster count never exceeds rune count, so it never misses a message that needs shortening.

`truncateSectionText` closes an unterminated fence afterwards. `Body.Block` is the method that carries long content, so it is the one truncation lands inside, and a fence left open breaks the rest of the rendering.

Config comes from the environment at the call site, never inside the package. `SLACK_WEBHOOK_URL` is the whole of it.

The payload carries no `username`, `icon_emoji` or `channel`, and there is no option to set them. An Incoming Webhook owned by a Slack app [cannot override](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks) any of the three — they always come from the app's own configuration, and no scope changes that. `WithUsername` / `WithIconEmoji` / `WithChannel` used to exist and were unused by all six services, which looked like an oversight until the reason surfaced: they only ever worked on legacy custom-integration webhooks, which Slack has deprecated. Documenting a knob that does nothing costs more than not having it. Per-service identity is a Slack-side matter — give each service its own app. Per-message identity would need `chat.postMessage` with `chat:write.customize`, a different API this package does not use.

If a genuinely effective option appears later (a footer toggle, say), adding `opts ...Option` back to `NewNotifier` does not break callers that pass none.

## Conventions

- Doc comments are Japanese, one per exported *and* unexported symbol, in the `名前 は …します。` form. Error strings are Japanese; wrap with `%w`.
- Constructors return `(*T, error)` and validate up front; where configuration is needed, use `type Option func(*T)` variadics.
- Tests are black-box (`package notify_test` / `package slack_test`) driving a stub `httpkit.Requester`. Use white-box `_internal_test.go` only for unexported helpers — `slack/blocks_internal_test.go` is the one such file, because the Markdown conversion rules are worth testing directly rather than through a webhook payload.
- Comments explain *why*, not *what*. The escaping-order and preserved-construct comments in `blocks.go` are the model: each states the failure it prevents.
