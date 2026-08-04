# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

**This repository contains no Go source yet** — only `go.mod` (`module github.com/shouni/go-notify`, go 1.26, zero dependencies), `README.md`, `LICENSE`, `.gitignore`. It is the extraction target for the notification layer currently living in the sibling repo `go-notifier` (`../go-notifier/pkg/slack`). Read that package before writing code here: this repo's `slack/` is meant to be that code, moved and made importable as a standalone library.

Consequently there is no CI, no `.golangci.yml`, and no test files here yet. The CI badge in `README.md` points at `shouni/go-notifier`'s workflow, not this repo's — a leftover to fix when CI is added.

## Commands

```bash
go build ./...                    # build
go vet ./...
gofmt -l .                        # must print nothing; `gofmt -w .` to fix
go test -race ./...               # full suite
go test -race ./slack/... -run TestName   # single test
golangci-lint run                 # sibling repos pin v2.12.2
govulncheck ./...
```

When adding CI, mirror `../go-notifier/.github/workflows/ci.yml`: three parallel jobs on push/PR to `main`/`develop` — build+vet+gofmt+`go test -race`, `golangci-lint@v2.12.2`, `govulncheck` — with the Go version read from `go.mod`.

Branches: work happens on `develop`; `main` is the release branch and tags drive the pkg.go.dev version.

## Intended architecture

Per `README.md`, two packages:

```
notify.go        # package notify: channel-agnostic notification interface + pipeline pattern
slack/           # package slack: Incoming Webhook implementation (Block Kit)
```

The design invariant across this family of repos is that **no package owns HTTP concerns**. All outbound traffic goes through `github.com/shouni/go-http-kit`'s `httpkit.Requester`, which is injected into constructors — it carries timeouts, exponential-backoff retry (`netarmor/retry`), SSRF/DNS-rebinding-safe dialing, and response-size limits. Package code therefore contains no retry loops, no `http.Client`, and no status-code handling; it builds a payload and calls `PostJSONAndFetchBytes`. Take `httpkit.Requester` (the narrow interface), not `*httpkit.Client`, so tests can substitute a fake.

### Slack package (as it exists in go-notifier/pkg/slack)

- `client.go` — `Client` holds `httpkit.Requester`, webhook URL, and the display overrides. `NewClient` rejects a nil client or empty webhook URL, seeds defaults (`Bot`, `:robot_face:`), then applies `Option`s. `SendTextWithHeader` builds the payload and posts it; `SendText` derives the header from the message's first line via `generateHeader` (trimmed, `📢 ` prefix, truncated to 50 runes with `...`, falling back to `📢 通知メッセージ` when empty).
- `options.go` — functional options `WithUsername`, `WithIconEmoji`, `WithChannel`. `WithChannel` overrides the channel baked into the webhook.
- `blocks.go` — Markdown → Slack Block Kit conversion (header block, section, footer context block) plus section-text truncation. Slack's per-block text limits are enforced here, not at the client layer.

Truncation is rune-based (`utf8.RuneCountInString` / `go-utils/text.Truncate`), never byte-based — messages are Japanese.

Config comes from the environment at the call site, not inside the package: `SLACK_WEBHOOK_URL` (required), `SLACK_USERNAME`, `SLACK_ICON_EMOJI`, `SLACK_CHANNEL`.

## Conventions

- Doc comments are written in Japanese, one per exported *and* unexported symbol, in the `名前 は …します。` form. Error strings returned to users are Japanese; wrap with `%w` (`fmt.Errorf("slack Webhookメッセージの送信に失敗しました: %w", err)`).
- Constructors return `(*T, error)` and validate arguments up front; configuration is applied via a `type Option func(*T)` variadic.
- Tests in the source repo are black-box (`package slack_test`) driving a stub `httpkit.Requester`; use white-box `_internal_test.go` files only when a test needs unexported helpers.
