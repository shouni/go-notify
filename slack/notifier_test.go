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

// blockSet は送信済みメッセージのブロック一式を返します。
// 種別付きの通知は attachment に包まれるため、両方の置き場所を見ます。
func (s *stubRequester) blockSet(t *testing.T) []slackgo.Block {
	t.Helper()
	if s.sent.Blocks != nil {
		return s.sent.Blocks.BlockSet
	}
	if len(s.sent.Attachments) == 1 {
		return s.sent.Attachments[0].Blocks.BlockSet
	}
	t.Fatal("Blocks も Attachments も設定されていません")
	return nil
}

// sectionText は送信済みメッセージのセクションブロック本文を返します。
func (s *stubRequester) sectionText(t *testing.T) string {
	t.Helper()
	for _, b := range s.blockSet(t) {
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

// TestNotifyRequiresTitle は、見出し未指定の通知が送信されずエラーになることを検証します。
//
// 本文の 1 行目から見出しを推測する機能は持ちません。何を見出しにするかは
// 通知の意味を決める判断で、本文の先頭行がそれである保証は無いためです。
func TestNotifyRequiresTitle(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Body: "先頭行\n二行目"}); err == nil {
		t.Error("Notify() = nil, want error")
	}
	if stub.sent.Text != "" {
		t.Errorf("見出し無しで送信されました: %q", stub.sent.Text)
	}
}

// TestNotifyAppliesOptions は、表示のカスタマイズがペイロードに反映されることを検証します。
func TestNotifyAppliesOptions(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test",
		slack.WithUsername("Bot Name"),
		slack.WithIconEmoji(":musical_note:"),
		slack.WithChannel("#notifications"),
	)
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	if stub.sent.Username != "Bot Name" {
		t.Errorf("Username = %q, want %q", stub.sent.Username, "Bot Name")
	}
	if stub.sent.IconEmoji != ":musical_note:" {
		t.Errorf("IconEmoji = %q, want %q", stub.sent.IconEmoji, ":musical_note:")
	}
	if stub.sent.Channel != "#notifications" {
		t.Errorf("Channel = %q, want %q", stub.sent.Channel, "#notifications")
	}
}

// TestNotifyLevelSetsAttachmentColor は、結果の種別が attachment の色になることを検証します。
func TestNotifyLevelSetsAttachmentColor(t *testing.T) {
	tests := []struct {
		name  string
		level notify.Level
		want  string
	}{
		{name: "成功は good", level: notify.LevelSuccess, want: "good"},
		{name: "失敗は danger", level: notify.LevelFailure, want: "danger"},
		{name: "スキップは warning", level: notify.LevelSkipped, want: "warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubRequester{}
			n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
			if err != nil {
				t.Fatalf("NewNotifier() = %v, want nil", err)
			}

			msg := notify.Message{Title: "件名", Body: "本文", Level: tt.level}
			if err := n.Notify(context.Background(), msg); err != nil {
				t.Fatalf("Notify() = %v, want nil", err)
			}

			if len(stub.sent.Attachments) != 1 {
				t.Fatalf("attachment 数 = %d, want 1", len(stub.sent.Attachments))
			}
			if got := stub.sent.Attachments[0].Color; got != tt.want {
				t.Errorf("Color = %q, want %q", got, tt.want)
			}
			if stub.sent.Blocks != nil {
				t.Error("attachment 使用時にトップレベル Blocks が設定されています")
			}
		})
	}
}

// TestNotifyWithoutLevelKeepsTopLevelBlocks は、種別未指定の通知が
// attachment に包まれず従来どおり投稿されることを検証します。
// 既存の通知の見た目を変えないための境界です。
func TestNotifyWithoutLevelKeepsTopLevelBlocks(t *testing.T) {
	stub := &stubRequester{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	if len(stub.sent.Attachments) != 0 {
		t.Errorf("attachment 数 = %d, want 0", len(stub.sent.Attachments))
	}
	if stub.sent.Blocks == nil {
		t.Error("トップレベル Blocks が設定されていません")
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
		Code("Command", "run_task").
		Field("Title", "サンプルタイトル").
		Link("History Detail", "https://example.com/web/history/job-1", "job-1")

	if err := n.Notify(context.Background(), notify.Message{Title: "✅ 完了", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	want := "*Command:* `run_task`\n" +
		"*Title:* サンプルタイトル\n" +
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

	body := notify.NewBody().Code("Job ID", "job-1").Error("エラー内容", errors.New("タイムアウトしました"))

	if err := n.Notify(context.Background(), notify.Message{Title: "❌ 失敗", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	got := stub.sectionText(t)
	if !strings.Contains(got, "*エラー内容:*\nタイムアウトしました") {
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
