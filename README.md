# 🔔 Go Notify

[![CI](https://github.com/shouni/go-notify/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-notify/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://go.dev/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-notify)](https://go.dev/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-notify)](https://github.com/shouni/go-notify/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-notify.svg)](https://pkg.go.dev/github.com/shouni/go-notify)

English | [日本語](README.ja.md)

## 🚀 About - Code that builds a body never has to know the channel

**Go Notify** is a Go library for notifying people of the results of asynchronous pipelines.
It has no CLI; you embed it in your application.

It is split into the channel-agnostic `notify` package and its implementation, the `slack`
package, so **the code that builds a body never has to know the channel**.

Slack-specific matters (markup conversion, retries, limits) are collected in [docs/slack.md](docs/slack.md).

## ✨ Features

* **Channel-agnostic body building**
  `notify.Body` emits standard Markdown, and the `slack` package converts it to Slack's own
  mrkdwn. Adding a new channel does not change callers.
* **Empty values are skipped automatically**
  Each `Body` method writes nothing when its value is empty, so no per-field `if` is needed.
* **An unset destination means "disabled", not an error**
  A blank webhook URL returns `notify.Disabled()`, and later calls silently succeed.
  Notifying is not an application's main purpose, and an unset destination is no reason to stop it from starting.
* **A robust transport**
  All outbound traffic goes through [`go-http-kit`](https://github.com/shouni/go-http-kit)'s
  `httpkit.Poster`. Timeouts, SSRF protection and the retry policy are those of the client
  you pass in; this package itself has neither retries nor an `http.Client`.

## 🚦 Usage

Install it with `go get github.com/shouni/go-notify`.

### Basics

```go
import (
    "github.com/shouni/go-notify/notify"
    "github.com/shouni/go-notify/slack"
)

// Webhook posts are not idempotent, so turning retries off is the safe choice (see docs/slack.md)
notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), os.Getenv("SLACK_WEBHOOK_URL"))
if err != nil {
    return err
}

body := notify.NewBody().
    Code("Command", task.Command).
    Code("Job ID", task.JobID).
    Link("History Detail", detailURL, task.JobID).
    Field("Title", task.Title)          // no line at all when Title is empty

err = notifier.Notify(ctx, notify.Message{
    Title: "✅ Generation finished",
    Body:  body.String(),
})
```

`Message.Title` is required. Sending it empty is an error
(no heading is guessed from the body's first line).

### Pipeline notifications

`notify.Pipeline` handles the fixed pattern where only the heading changes between success, failure and skip.

```go
pipeline := notify.NewPipeline(notifier, notify.Titles{
    Success: "✅ Generation finished",
    Failure: "❌ Generation failed",
    Skipped: "⏭️ Skipped: nothing changed",
})

// Bail out early when building the body is expensive
if !pipeline.Enabled() {
    return nil
}

body := notify.NewBody().Code("Job ID", jobID)

pipeline.Success(ctx, body)                 // body as is
pipeline.Failure(ctx, body, err)            // appends 「エラー内容」 (error) to the end of the body
pipeline.Skipped(ctx, body, reason)         // appends 「理由」 (reason) when reason is non-nil
```

`Failure` and `Skipped` append to a copy, so **the `Body` you pass in is not modified**.
Passing one `Body` again for each outcome, as above, does not leave the earlier
「エラー内容」 in the skipped notification.

The two section labels, 「エラー内容」 and 「理由」, are Japanese: they are what the library's owner reads in Slack.

To switch headings on a run-time condition (different wording per command, say), derive a
`Pipeline` with only the headings replaced through `WithTitles`.
The original `Pipeline` is not modified, so you can keep one and switch per call.

```go
pipeline.WithTitles(notify.Titles{Success: titleFor(cmd)}).Success(ctx, body)
```

### Writing a Body

Each method appends one line of Markdown (see the godoc for what each emits).
**A line with an empty value is not written at all.** That is what saves you lining up
one "write this field if present" `if` per field. `Error` / `Block` show `N/A` when there is no value.
`String()` on a `Body` with nothing written returns `N/A`.
`Heading` / `Error` / `Block` leave a blank line first when the body already has content.

For a variable number of values use `Bullet` rather than repeating `Field`, and use
`Heading` when many fields read better split into groups.
There is no need to hand-write `- ` or `## ` through `Text`.

The content of a `Block` is exempt from each channel's markup conversion, so `- ` and `**` are not rewritten
(details in [docs/slack.md](docs/slack.md)).

### Building link URLs

`Link` omits the whole line when the URL is empty. `notify.JoinURL` builds URLs to match
that contract. It returns an empty string when the base is empty, a path element is empty,
or the result cannot be parsed as a URL, so callers need no branch deciding whether to write the link line.

```go
body.Link("History Detail", notify.JoinURL(serviceURL, "/web/history", jobID), jobID)
// empty serviceURL or jobID → empty URL → no line at all
```

Path elements are URL-escaped (slashes stay as separators, so you can pass
a path such as `"/web/history"` as is).

### Making a value a code span

`Code` builds only "label + one value". To add a unit or an emoji, or to put
several code spans on one line, use `CodeSpan`.

It is the way out that keeps knowledge of Markdown inside the `notify` package,
so do not write backticks directly in calling code.

```go
body.Field("Seed", notify.CodeSpan(strconv.Itoa(seed))+" 🎲")
body.Field("Branch", notify.CodeSpan(base)+" ← "+notify.CodeSpan(feature))
```

### Outcome level (Level)

`Message.Level` carries the kind of outcome. `Pipeline` sets it automatically,
so callers do not need to.

How to present it is up to each channel (Slack uses an attachment's colour bar).
You no longer need `✅` `❌` in the heading to show the outcome, though you may keep them.
A `Message` built by hand stays at `LevelNone`, so its appearance does not change.

## 📦 Package Structure

| Package | Role |
| :--- | :--- |
| [`notify`](https://pkg.go.dev/github.com/shouni/go-notify/notify) | The channel-agnostic abstraction: `Notifier` / `Message` / `Body` / `Pipeline` |
| [`slack`](https://pkg.go.dev/github.com/shouni/go-notify/slack) | Slack Incoming Webhook implementation → [docs/slack.md](docs/slack.md) |

`slack` depends on `notify`, never the reverse. Adding a new channel means adding a subpackage
that implements `notify.Notifier`; `notify` itself needs no change.

## 📜 License

This project is released under the [MIT License](https://opensource.org/licenses/MIT).
