package slack

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// TestPayloadWireShape は、Incoming Webhook に送る JSON の形を固定します。
//
// この形は slack-go の構造体で組んでいたころのものと同じです（依存を外すときに
// 出力を突き合わせて確かめました）。差は、あちらが omitempty 無しで常に出していた
// replace_original / delete_original が無いことだけで、Webhook ではどちらも
// 意味を持ちません。ブロックの種類やキーの綴りを変えると Slack が invalid_blocks を
// 返して通知が丸ごと失われるので、ここで固定します。
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
