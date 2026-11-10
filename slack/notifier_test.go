package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/shouni/go-http-kit/httpkit"
	"github.com/shouni/go-notify/notify"
	"github.com/shouni/go-notify/slack"
)

// sentMessage is the sent payload read back in the JSON shape Slack
// receives. Tests check the wire shape rather than the library's internal
// types, so replacing those types does not change the tests.
type sentMessage struct {
	Text        string `json:"text"`
	Attachments []struct {
		Color    string      `json:"color"`
		Fallback string      `json:"fallback"`
		Blocks   []sentBlock `json:"blocks"`
	} `json:"attachments"`
	Blocks []sentBlock `json:"blocks"`
}

type sentBlock struct {
	Type string `json:"type"`
	Text *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"text"`
}

// stubPoster is an httpkit.Poster stub that records the sent payload.
type stubPoster struct {
	sent sentMessage
	raw  []byte
	fail error
}

// PostJSON records the sent payload after converting it to JSON.
func (s *stubPoster) PostJSON(_ context.Context, _ string, data any) (*httpkit.Result, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	s.raw = raw
	if err := json.Unmarshal(raw, &s.sent); err != nil {
		return nil, err
	}
	if s.fail != nil {
		return nil, s.fail
	}
	return &httpkit.Result{Status: http.StatusOK, Body: []byte("ok")}, nil
}

func (s *stubPoster) Post(_ context.Context, _, _ string, _ []byte) (*httpkit.Result, error) {
	return nil, nil
}

// blockSet returns the sent message's blocks. A notification with a level
// is wrapped in an attachment, so both places are checked.
func (s *stubPoster) blockSet(t *testing.T) []sentBlock {
	t.Helper()
	if len(s.sent.Blocks) > 0 {
		return s.sent.Blocks
	}
	if len(s.sent.Attachments) == 1 {
		return s.sent.Attachments[0].Blocks
	}
	t.Fatal("neither Blocks nor Attachments is set")
	return nil
}

// headerText returns the text of the sent message's header block.
func (s *stubPoster) headerText(t *testing.T) string {
	t.Helper()
	for _, b := range s.blockSet(t) {
		if b.Type == "header" && b.Text != nil {
			return b.Text.Text
		}
	}
	t.Fatal("header block not found")
	return ""
}

// sectionText returns the text of the sent message's first section block.
func (s *stubPoster) sectionText(t *testing.T) string {
	t.Helper()
	for _, b := range s.blockSet(t) {
		if b.Type == "section" && b.Text != nil {
			return b.Text.Text
		}
	}
	t.Fatal("section block not found")
	return ""
}

// TestNewNotifierDisabledOnBlankWebhookURL verifies that an unset webhook
// URL yields a disabled Notifier rather than an error. An unset
// destination is no reason to stop an application from starting.
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

// TestNewNotifierRequiresClientWhenConfigured verifies that a configured
// webhook URL with a nil HTTP client is an error.
func TestNewNotifierRequiresClientWhenConfigured(t *testing.T) {
	if _, err := slack.NewNotifier(nil, "https://hooks.slack.com/services/test"); err == nil {
		t.Error("NewNotifier() = nil, want error")
	}
}

// TestNewNotifierEnabled verifies that a complete configuration yields a
// Notifier that can send.
func TestNewNotifierEnabled(t *testing.T) {
	n, err := slack.NewNotifier(&stubPoster{}, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}
	if !notify.Enabled(n) {
		t.Error("Enabled() = false, want true")
	}
}

// TestNotifyUsesTitleAsHeader verifies that Message.Title becomes the
// heading.
func TestNotifyUsesTitleAsHeader(t *testing.T) {
	stub := &stubPoster{}
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

// TestNotifyTruncatesLongTitle verifies that an overlong heading is cut to
// fit the header block limit.
//
// Slack's header block allows 150 characters; sent over that, Slack answers
// invalid_blocks and the whole notification is lost. The heading needs
// protecting as much as the body.
func TestNotifyTruncatesLongTitle(t *testing.T) {
	stub := &stubPoster{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	title := strings.Repeat("あ", 200)
	if err := n.Notify(context.Background(), notify.Message{Title: title, Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	if got := len([]rune(stub.headerText(t))); got > 150 {
		t.Errorf("header length = %d, want <= 150", got)
	}
}

// TestNotifyRequiresTitle verifies that a notification without a heading is
// not sent and returns an error.
//
// There is no guessing a heading from the body's first line. Choosing the
// heading decides what the notification means, and nothing guarantees the
// first line is it.
func TestNotifyRequiresTitle(t *testing.T) {
	stub := &stubPoster{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Body: "先頭行\n二行目"}); err == nil {
		t.Error("Notify() = nil, want error")
	}
	if stub.sent.Text != "" {
		t.Errorf("sent without a heading: %q", stub.sent.Text)
	}
}

// TestNotifyLevelSetsAttachmentColor verifies that the outcome level becomes
// the attachment colour.
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
			stub := &stubPoster{}
			n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
			if err != nil {
				t.Fatalf("NewNotifier() = %v, want nil", err)
			}

			msg := notify.Message{Title: "件名", Body: "本文", Level: tt.level}
			if err := n.Notify(context.Background(), msg); err != nil {
				t.Fatalf("Notify() = %v, want nil", err)
			}

			if len(stub.sent.Attachments) != 1 {
				t.Fatalf("attachment count = %d, want 1", len(stub.sent.Attachments))
			}
			if got := stub.sent.Attachments[0].Color; got != tt.want {
				t.Errorf("Color = %q, want %q", got, tt.want)
			}
			if len(stub.sent.Blocks) > 0 {
				t.Error("top-level Blocks set while using an attachment")
			}
		})
	}
}

// TestNotifyLevelDoesNotDuplicateTitle verifies that a coloured notification
// does not show its heading twice.
//
// With the blocks inside an attachment, top-level Text is rendered as the
// message body rather than as a fallback. Leaving the heading in Text puts
// it in Slack twice, alongside the attachment's header block.
// Attachment.Fallback handles the fallback, so push notification wording
// remains.
func TestNotifyLevelDoesNotDuplicateTitle(t *testing.T) {
	stub := &stubPoster{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	msg := notify.Message{Title: "✅ 完了しました", Body: "本文", Level: notify.LevelSuccess}
	if err := n.Notify(context.Background(), msg); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	if stub.sent.Text != "" {
		t.Errorf("Text = %q, want empty (it duplicates the heading inside the attachment)", stub.sent.Text)
	}
	if len(stub.sent.Attachments) != 1 {
		t.Fatalf("attachment count = %d, want 1", len(stub.sent.Attachments))
	}
	if got := stub.sent.Attachments[0].Fallback; got != msg.Title {
		t.Errorf("Fallback = %q, want %q", got, msg.Title)
	}
}

// TestNotifyWithoutLevelKeepsTopLevelBlocks verifies that a notification
// without a level is not wrapped in an attachment and posts as before. It is
// the boundary that keeps existing notifications looking the same.
func TestNotifyWithoutLevelKeepsTopLevelBlocks(t *testing.T) {
	stub := &stubPoster{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	if len(stub.sent.Attachments) != 0 {
		t.Errorf("attachment count = %d, want 0", len(stub.sent.Attachments))
	}
	if len(stub.sent.Blocks) == 0 {
		t.Error("top-level Blocks not set")
	}
}

// TestNotifyPropagatesSendError verifies that a send failure reaches the
// caller.
func TestNotifyPropagatesSendError(t *testing.T) {
	stub := &stubPoster{fail: errors.New("network error")}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	if err := n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err == nil {
		t.Error("Notify() = nil, want error")
	}
}

// TestNotifyRedactsWebhookURLInSendError verifies that the webhook URL does
// not appear in a send failure's text. The URL is itself the authorisation
// to post, HTTP clients put the destination into failure text, and callers
// log that error as is. Checking the cause (errors.Is) must still work after
// redaction.
func TestNotifyRedactsWebhookURLInSendError(t *testing.T) {
	const webhookURL = "https://hooks.slack.com/services/T000/B000/SECRETTOKEN"
	cause := errors.New("connection refused")
	// The shape httpkit and net/http return: the URL appears twice in the text.
	stub := &stubPoster{fail: fmt.Errorf("HTTP request failed (URL: %s): %w",
		webhookURL, &url.Error{Op: "Post", URL: webhookURL, Err: cause})}
	n, err := slack.NewNotifier(stub, webhookURL)
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	err = n.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"})
	if err == nil {
		t.Fatal("Notify() = nil, want error")
	}
	if strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Errorf("Notify() error leaks the webhook URL: %v", err)
	}
	if !strings.Contains(err.Error(), "<webhook URL>") {
		t.Errorf("Notify() error does not mark the redacted URL: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("Notify() error lost the cause: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false; Unwrap must return the original error")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Errorf("errors.As(err, *url.Error) = false; Unwrap must return the original error")
	}
}

// TestNotifyRendersBodyAsMrkdwn verifies that the generic Markdown built by
// notify.Body is converted to Slack mrkdwn before sending. It is the very
// boundary that keeps the notify package channel-agnostic.
func TestNotifyRendersBodyAsMrkdwn(t *testing.T) {
	stub := &stubPoster{}
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
		t.Errorf("section text =\n%q\nwant\n%q", got, want)
	}
}

// TestNotifyRendersHeadingAndBulletAsMrkdwn verifies that Heading and Bullet
// are converted to Slack's markup.
//
// Both methods exist so that callers need not write "## " or "- " by hand,
// so the path by which Body's output is actually converted is pinned here.
func TestNotifyRendersHeadingAndBulletAsMrkdwn(t *testing.T) {
	stub := &stubPoster{}
	n, err := slack.NewNotifier(stub, "https://hooks.slack.com/services/test")
	if err != nil {
		t.Fatalf("NewNotifier() = %v, want nil", err)
	}

	body := notify.NewBody().
		Field("Title", "サンプル").
		Heading("生成結果").
		Bullet("scene_01.png").
		Bullet("scene_02.png")

	if err := n.Notify(context.Background(), notify.Message{Title: "✅ 完了", Body: body.String()}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	want := "*Title:* サンプル\n\n" +
		"*生成結果*\n" +
		"• scene_01.png\n" +
		"• scene_02.png"
	if got := stub.sectionText(t); got != want {
		t.Errorf("section text =\n%q\nwant\n%q", got, want)
	}
}

// TestNotifyRendersErrorBlockAsMrkdwn verifies that the error section is
// converted to mrkdwn.
func TestNotifyRendersErrorBlockAsMrkdwn(t *testing.T) {
	stub := &stubPoster{}
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
		t.Errorf("error section not converted to mrkdwn: %q", got)
	}
}

// TestNotifyKeepsCodeBlockContentVerbatim verifies that content passed to
// notify.Body.Block reaches Slack unrewritten.
//
// Block is the way in for showing command output or logs verbatim, so the
// asymmetry is correct here: the label is converted to mrkdwn while the
// block's content passes through untouched.
func TestNotifyKeepsCodeBlockContentVerbatim(t *testing.T) {
	stub := &stubPoster{}
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
		t.Errorf("section text =\n%q\nwant\n%q", got, want)
	}
}
