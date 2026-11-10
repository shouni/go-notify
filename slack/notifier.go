package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shouni/go-http-kit/httpkit"
	"github.com/shouni/go-notify/notify"
)

// levelColors maps each outcome level to a Slack attachment colour.
// good / danger / warning are Slack's built-in keywords, and an empty string
// means "no colour".
//
// LevelNone has an entry too, so that every level is listed and exhaustive
// catches a level added without one. A missing key would look up as an
// empty string, pass silently, and post uncoloured.
var levelColors = map[notify.Level]string{
	notify.LevelNone:    "",
	notify.LevelSuccess: "good",
	notify.LevelFailure: "danger",
	notify.LevelSkipped: "warning",
}

// notifier posts notifications to a Slack Incoming Webhook.
//
// It sets no poster name, icon or destination channel: an Incoming Webhook
// created through a Slack app refuses to override them and always inherits
// the app's settings (only the custom-integration kind, which Slack has
// deprecated, accepted them). To present each service differently, give
// each its own Slack app.
type notifier struct {
	client     httpkit.Poster
	webhookURL string
}

// Ensure notifier implements notify.Notifier.
var _ notify.Notifier = (*notifier)(nil)

// NewNotifier creates a notify.Notifier for a Slack Incoming Webhook.
//
// A blank webhookURL means notifications are not configured, and it returns
// notify.Disabled(). A configured URL with a nil client is a configuration
// mistake that cannot send, so it returns an error.
//
// # Retries
//
// Webhook posts are not idempotent: if the post reaches Slack but the
// response is lost, a retry posts the same notification twice. httpkit.New
// enables retry by default, so this can happen when the client is shared
// with other traffic. Whether a lost notification or a duplicate is worse
// is the application's decision, so this package takes no position and uses
// the client it is given as is.
//
//	notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), webhookURL)
func NewNotifier(client httpkit.Poster, webhookURL string) (notify.Notifier, error) {
	if strings.TrimSpace(webhookURL) == "" {
		return notify.Disabled(), nil
	}
	if client == nil {
		return nil, errors.New("slack webhook URL is set but the HTTP client is nil")
	}

	return &notifier{client: client, webhookURL: webhookURL}, nil
}

// Notify implements the notify.Notifier interface. msg.Title is required.
// It does not guess a heading from the body, because choosing the heading
// decides what the notification means, and nothing guarantees the body's
// first line is it.
func (n *notifier) Notify(ctx context.Context, msg notify.Message) error {
	payload, err := n.buildWebhookMessage(ctx, msg)
	if err != nil {
		return err
	}

	if _, err := n.client.PostJSON(ctx, n.webhookURL, payload); err != nil {
		return &sendError{webhookURL: n.webhookURL, cause: err}
	}

	return nil
}

// redactedWebhookURL is written in place of the webhook URL in error text.
const redactedWebhookURL = "<webhook URL>"

// sendError reports a send failure with the webhook URL redacted from its
// text.
//
// An Incoming Webhook URL is itself the authorisation to post, so it is
// treated as a secret. HTTP clients, however, put the destination into
// their failure text (both net/http's *url.Error and httpkit's
// "HTTP request failed (URL: …)"). Callers log this error as is, so unless
// it is redacted here the secret ends up in the logging backend.
//
// Unwrap returns the original error, so errors.Is / errors.As still work on
// the cause. Only the Error() text is redacted.
type sendError struct {
	webhookURL string
	cause      error
}

func (e *sendError) Error() string {
	return "failed to send slack webhook message: " +
		strings.ReplaceAll(e.cause.Error(), e.webhookURL, redactedWebhookURL)
}

func (e *sendError) Unwrap() error { return e.cause }

// buildWebhookMessage builds the payload sent to the Slack Incoming Webhook.
//
// Only levels with a colour are wrapped in an attachment. An attachment adds
// a colour bar on the left but indents the body, so LevelNone stays as
// top-level blocks to keep level-less notifications looking as they did.
func (n *notifier) buildWebhookMessage(ctx context.Context, msg notify.Message) (webhookMessage, error) {
	blocks, err := buildMessageBlocks(ctx, msg.Title, msg.Body)
	if err != nil {
		return webhookMessage{}, fmt.Errorf("failed to build slack Block Kit: %w", err)
	}

	var payload webhookMessage

	// A level missing from the table (a numeric conversion, say) looks up as
	// an empty string and is treated like LevelNone.
	color := levelColors[msg.Level]
	if color == "" {
		// With top-level blocks, Text is not rendered as the body; it serves
		// only as the fallback for push notifications and the like.
		payload.Text = msg.Title
		payload.Blocks = blocks
		return payload, nil
	}

	// When wrapping in an attachment, leave Text empty. With the blocks inside
	// the attachment, Text is rendered as the message body rather than as a
	// fallback, so together with the attachment's header block the heading
	// would appear twice. Attachment.Fallback takes over the fallback role,
	// so push notification wording is not lost.
	payload.Attachments = []attachment{{
		Color:    color,
		Fallback: msg.Title,
		Blocks:   blocks,
	}}
	return payload, nil
}
