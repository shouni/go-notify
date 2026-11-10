# Slack channel

The `slack` package is the Slack Incoming Webhook implementation of `notify.Notifier`.
For channel-agnostic usage, see the [README](../README.md).

```go
notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), os.Getenv("SLACK_WEBHOOK_URL"))
```

`NewNotifier` is the only export. The posting itself is unexported, and
callers treat the result as a `notify.Notifier`.

---

## ⚠️ Retries

**Webhook posts are not idempotent.** Every successful one creates a new message, so if a
post reaches Slack but the response is lost and the request is retried, the same notification is posted twice.

This library has no retry policy of its own and uses the `httpkit.Poster` it is given as is.
`httpkit.New(timeout)` enables retry by default, so **sharing one client with other traffic
can produce duplicate posts**.

Deriving a client with `WithoutRetry` turns retries off for the notification path only, while
still sharing the existing client's timeout, SSRF protection and connection pool.

```go
notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), webhookURL)
```

If no part of the client needs retries, `httpkit.New(timeout, httpkit.WithNoRetry())` works too.

Whether a lost notification or a duplicate post is worse is the application's decision,
so the library does not make it.

---

## 🎨 Markup conversion

The standard Markdown that `Body` emits is converted to Slack mrkdwn.

* `**bold**` → `*bold*`, `## heading` → `*heading*`, `- item` → `• item`
* `[text](URL)` → `<URL|text>`
* `&` `<` `>` in plain text are escaped to entities
  (so `<nil>` in an error string is not mistaken for link syntax)

Blockquotes with `>` at the start of a line are unsupported, because they cannot be told apart from a `>` that needs escaping.

A link URL may contain one level of balanced parentheses
(`https://example.com/a_(b)_c` becomes a single link).
A URL with unbalanced parentheses is not converted and stays as Markdown,
because seeing it as is reads better than a broken link.

### Where nothing is escaped

Escaping applies to **plain text only**. The inside of `<URL|text>` and mentions such as
`<@U123>` are already parsed by Slack as syntax, so they are left alone.
Escaping the `&` in a signed URL changes the signature and yields a 403,
so this boundary is deliberate.

### Where nothing is converted

The content of a code block enclosed in fences (` ``` ` at the start of a line) is exempt from markup conversion.
`Body.Block` exists to show command output or logs verbatim, so rewriting `- ` to
`• ` or `**text**` to `*text*` there would break the very text its author wants to see.

Escaping `&` `<` `>` is still needed inside blocks, so it is applied separately from the markup conversion.

---

## 🎨 Showing the outcome level (Level)

`notify.Message.Level` becomes the colour bar of an attachment.

| Level | Colour |
| :--- | :--- |
| `LevelSuccess` | `good` (green) |
| `LevelFailure` | `danger` (red) |
| `LevelSkipped` | `warning` (yellow) |
| `LevelNone` (zero value) | none (stays as top-level blocks) |

`LevelNone` is not wrapped in an attachment because an attachment indents the body slightly.
There is no reason to change how notifications without a level look.

---

## 🕒 Footer

Every notification automatically ends with a context block showing the send time (JST),
labelled 「送信時刻」. Callers do not need to put the time in the body.

---

## 📏 Limits

* **A heading is required.** An empty `Message.Title` returns an error.
* The heading is truncated at **150 characters**, the header block limit. That is Slack's
  own limit, and sending over it returns `invalid_blocks` and loses the whole notification.
* The body is split into section blocks of up to **2900 characters** each
  (leaving a margin under Slack's limit of 3000), and only truncated when it would
  need more than **20 blocks**: the 20th block is cut and ends with the truncation note.
* Truncation is by grapheme cluster. Cutting by rune would split characters
  with a dakuten or ZWJ emoji midway.
* When truncation happens inside a code block, a closing fence is added,
  because a fence left open breaks the rendering of everything after it.
* The poster's display name, icon and destination channel cannot be set. An Incoming Webhook
  created through a Slack app always takes these from the app's own settings.
