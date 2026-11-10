package slack

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// TestPayloadWireShape pins the shape of the JSON sent to the Incoming
// Webhook.
//
// The shape is the same as when it was built from slack-go's structs (the
// outputs were compared when the dependency was removed). The only
// difference is the absence of replace_original / delete_original, which
// slack-go always emitted without omitempty and which mean nothing on a
// webhook. Changing a block type or a key's spelling makes Slack answer
// invalid_blocks and the whole notification is lost, so it is pinned here.
func TestPayloadWireShape(t *testing.T) {
	t.Parallel()

	n := &notifier{webhookURL: "x"}
	msg := notify.Message{Title: "件名 <a&b>", Body: "本文 **強調**\n- item\n```\ncode\n```"}
	timestamp := regexp.MustCompile(`送信時刻: [^"]*`)

	tests := map[notify.Level]string{
		notify.LevelNone:    `{"text":"件名 \u003ca\u0026b\u003e","blocks":[{"type":"header","text":{"type":"plain_text","text":"件名 \u003ca\u0026b\u003e","emoji":true}},{"type":"divider"},{"type":"section","text":{"type":"mrkdwn","text":"本文 *強調*\n• item\n` + "```" + `\ncode\n` + "```" + `"}},{"type":"context","block_id":"notification-context","elements":[{"type":"mrkdwn","text":"送信時刻: TS"}]}]}`,
		notify.LevelSuccess: `{"attachments":[{"color":"good","fallback":"件名 \u003ca\u0026b\u003e","blocks":[{"type":"header","text":{"type":"plain_text","text":"件名 \u003ca\u0026b\u003e","emoji":true}},{"type":"divider"},{"type":"section","text":{"type":"mrkdwn","text":"本文 *強調*\n• item\n` + "```" + `\ncode\n` + "```" + `"}},{"type":"context","block_id":"notification-context","elements":[{"type":"mrkdwn","text":"送信時刻: TS"}]}]}]}`,
		notify.LevelSkipped: `{"attachments":[{"color":"warning","fallback":"件名 \u003ca\u0026b\u003e","blocks":[{"type":"header","text":{"type":"plain_text","text":"件名 \u003ca\u0026b\u003e","emoji":true}},{"type":"divider"},{"type":"section","text":{"type":"mrkdwn","text":"本文 *強調*\n• item\n` + "```" + `\ncode\n` + "```" + `"}},{"type":"context","block_id":"notification-context","elements":[{"type":"mrkdwn","text":"送信時刻: TS"}]}]}]}`,
		notify.LevelFailure: `{"attachments":[{"color":"danger","fallback":"件名 \u003ca\u0026b\u003e","blocks":[{"type":"header","text":{"type":"plain_text","text":"件名 \u003ca\u0026b\u003e","emoji":true}},{"type":"divider"},{"type":"section","text":{"type":"mrkdwn","text":"本文 *強調*\n• item\n` + "```" + `\ncode\n` + "```" + `"}},{"type":"context","block_id":"notification-context","elements":[{"type":"mrkdwn","text":"送信時刻: TS"}]}]}]}`,
	}

	for level, want := range tests {
		msg.Level = level
		payload, err := n.buildWebhookMessage(context.Background(), msg)
		if err != nil {
			t.Fatalf("level %v: %v", level, err)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("level %v: %v", level, err)
		}
		if got := timestamp.ReplaceAllString(string(raw), "送信時刻: TS"); got != want {
			t.Errorf("level %v:\n got %s\nwant %s", level, got, want)
		}
	}
}
