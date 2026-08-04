package slack_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/shouni/go-notify/notify"
	"github.com/shouni/go-notify/slack"
	slackgo "github.com/slack-go/slack"
)

// stubRequester は送信ペイロードを記録する httpkit.Requester のスタブです。
type stubRequester struct {
	sent slackgo.WebhookMessage
	fail error
}

// PostJSONAndFetchBytes は送信された WebhookMessage を記録します。
func (s *stubRequester) PostJSONAndFetchBytes(_ context.Context, _ string, data any) ([]byte, error) {
	if msg, ok := data.(slackgo.WebhookMessage); ok {
		s.sent = msg
	}
	return []byte("ok"), s.fail
}

func (s *stubRequester) DoRequest(_ *http.Request) ([]byte, error) { return nil, nil }
func (s *stubRequester) FetchBytes(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", nil
}
func (s *stubRequester) FetchAndDecodeJSON(_ context.Context, _ string, _ any) error { return nil }
func (s *stubRequester) PostRawBodyAndFetchBytes(_ context.Context, _ string, _ []byte, _ string) ([]byte, error) {
	return nil, nil
}

// sectionText は送信済みメッセージのセクションブロック本文を返します。
func (s *stubRequester) sectionText(t *testing.T) string {
	t.Helper()
	if s.sent.Blocks == nil {
		t.Fatal("Blocks が設定されていません")
	}
	for _, b := range s.sent.Blocks.BlockSet {
		if section, ok := b.(*slackgo.SectionBlock); ok && section.Text != nil {
			return section.Text.Text
		}
	}
	t.Fatal("セクションブロックが見つかりません")
	return ""
}

// TestNewNotifierDisabledOnBlankWebhookURL は、Webhook URL が未設定なら
// エラーではなく無効な Notifier になることを検証します。
// 通知先の未設定はアプリケーションの起動を妨げる理由にならないためです。
func TestNewNotifierDisabledOnBlankWebhookURL(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
	}{
		{name: "空文字", webhookURL: ""},
		{name: "空白のみ", webhookURL: "   "},
		{name: "改行のみ", webhookURL: "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := slack.NewNotifier(nil, tt.webhookURL)
			if err != nil {
				t.Fatalf("NewNotifier() = %v, want nil", err)
			}
			if notify.Enabled(n) {
				t.Error("Enabled() = true, want false")
			}
			if err := n.Notify(context.Background(), notify.Message{Title: "件名"}); err != nil {
				t.Errorf("Notify() = %v, want nil", err)
			}
		})
	}
}

// TestNewNotifierRequiresClientWhenConfigured は、Webhook URL があるのに
// HTTP クライアントが nil の場合はエラーになることを検証します。
func TestNewNotifierRequiresClientWhenConfigured(t *testing.T) {
	if _, err := slack.NewNotifier(nil, "https://hooks.slack.com/services/test"); err == nil {
		t.Error("NewNotifier() = nil, want error")
	}
}

// TestNewNotifierEnabled は、設定が揃っていれば送信可能な Notifier になることを検証します。
func TestNewNotifierEnabled(t *testing.T) {
	n, err := slack.NewNotifier(&stubRequester{}, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}
	if !notify.Enabled(n) {
		t.Error("Enabled() = false, want true")
	}
}

// TestNotifyUsesTitleAsHeader は Message.Title が見出しになることを検証します。
func TestNotifyUsesTitleAsHeader(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "✅ 完了しました", Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if stub.sent.Text != "✅ 完了しました" {
		t.Errorf("Text = %q, want %q", stub.sent.Text, "✅ 完了しました")
	}
}

// TestNotifyGeneratesHeaderWhenTitleEmpty は、見出し未指定時に本文の
// 先頭行から見出しが生成されることを検証します。
func TestNotifyGeneratesHeaderWhenTitleEmpty(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Body: "先頭行\n二行目"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if want := "📢 先頭行"; stub.sent.Text != want {
		t.Errorf("Text = %q, want %q", stub.sent.Text, want)
	}
}

// TestNotifyPropagatesSendError は送信失敗が呼び出し元へ伝わることを検証します。
func TestNotifyPropagatesSendError(t *testing.T) {
	stub := &stubRequester{fail: errors.New("network error")}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err == nil {
		t.Error("Notify() = nil, want error")
	}
}

// TestNotifyRendersBodyAsMrkdwn は、notify.Body が組み立てた汎用 Markdown が
// Slack mrkdwn に変換されて送信されることを検証します。
// notify パッケージをチャネル非依存に保つための境界そのものです。
func TestNotifyRendersBodyAsMrkdwn(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	body := notify.NewBody().
		Code("Command", "compose_video").
		Field("Title", "夏の終わり").
		Link("History Detail", "https://example.com/web/history/job-1", "job-1")

	if err := n.Notify(context.Background(), notify.Message{Title: "✅ 完了", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	want := "*Command:* `compose_video`\n" +
		"*Title:* 夏の終わり\n" +
		"*History Detail:* <https://example.com/web/history/job-1|job-1>"
	if got := stub.sectionText(t); got != want {
		t.Errorf("セクション本文 =\n%q\nwant\n%q", got, want)
	}
}

// TestNotifyRendersErrorBlockAsMrkdwn は、エラー節がコードブロックのまま
// 変換されることを検証します。
func TestNotifyRendersErrorBlockAsMrkdwn(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	body := notify.NewBody().Code("Job ID", "job-1").Error("エラー内容", errors.New("Veo がタイムアウト"))

	if err := n.Notify(context.Background(), notify.Message{Title: "❌ 失敗", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	got := stub.sectionText(t)
	if !strings.Contains(got, "*エラー内容:*\nVeo がタイムアウト") {
		t.Errorf("エラー節が mrkdwn に変換されていません: %q", got)
	}
}

// TestNotifyKeepsCodeBlockContentVerbatim は、notify.Body.Block に渡した内容が
// Slack へ届くまで書き換えられないことを検証します。
//
// Block はコマンド出力やログを原文のまま見せるための入口なので、
// ラベル側が mrkdwn に変換される一方でブロックの中身は素通しになる、
// という非対称がここでの正しい振る舞いです。
func TestNotifyKeepsCodeBlockContentVerbatim(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	body := notify.NewBody().
		Code("Job ID", "job-1").
		Block("実行ログ", "usage: cmd [options]\n  - --flag  **必須**")

	if err := n.Notify(context.Background(), notify.Message{Title: "❌ 失敗", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	want := "*Job ID:* `job-1`\n\n" +
		"*実行ログ:*\n" +
		"```\nusage: cmd [options]\n  - --flag  **必須**\n```"
	if got := stub.sectionText(t); got != want {
		t.Errorf("セクション本文 =\n%q\nwant\n%q", got, want)
	}
}
