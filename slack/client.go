package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shouni/go-http-kit/httpkit"
	"github.com/shouni/go-utils/text"
	"github.com/slack-go/slack"
)

const (
	// defaultUsername はデフォルトのユーザー名です。
	defaultUsername = "Bot"
	// defaultIconEmoji はデフォルトの絵文字アイコンを表します。
	defaultIconEmoji = ":robot_face:"
	// defaultHeader は本文からヘッダーを作れない場合に使うヘッダーです。
	defaultHeader = "📢 通知メッセージ"
	// headerPrefix は自動生成ヘッダーに付与する接頭辞です。
	headerPrefix = "📢 "
	// maxHeaderLength は本文から抽出するヘッダーの最大文字数です。
	maxHeaderLength = 50
	// truncationEllipsis は短いテキストを切り捨てるときに追加するサフィックスです。
	truncationEllipsis = "..."
)

// Client は Slack Webhook API と連携するためのクライアントです。
type Client struct {
	client     httpkit.Requester
	webhookURL string
	username   string
	iconEmoji  string
	channel    string
}

// NewClient は Slack Webhook API クライアントを生成します。
// opts を指定すると、ユーザー名、アイコン絵文字、送信先チャンネルを上書きできます。
//
// client のリトライ方針はそのまま使われます。Webhook への投稿は非冪等なため、
// 重複投稿を避けたい場合はリトライを無効化したクライアントを渡してください
// （詳細は NewNotifier のドキュメントを参照）。
//
// webhookURL が空の場合はエラーを返します。通知先が任意設定であることを
// 表現したい場合は、NewNotifier を使ってください。
func NewClient(client httpkit.Requester, webhookURL string, opts ...Option) (*Client, error) {
	if client == nil {
		return nil, errors.New("http client cannot be nil")
	}
	if webhookURL == "" {
		return nil, errors.New("webhookURL is required")
	}

	c := &Client{
		client:     client,
		webhookURL: webhookURL,
		username:   defaultUsername,
		iconEmoji:  defaultIconEmoji,
	}

	// オプションを適用
	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// SendTextWithHeader は指定されたヘッダーと本文から Slack メッセージを構築して送信します。
func (s *Client) SendTextWithHeader(ctx context.Context, headerText string, message string) error {
	msg, err := s.buildWebhookMessage(headerText, message)
	if err != nil {
		return err
	}

	_, err = s.client.PostJSONAndFetchBytes(ctx, s.webhookURL, msg)
	if err != nil {
		return fmt.Errorf("slack Webhookメッセージの送信に失敗しました: %w", err)
	}

	return nil
}

// SendText は本文の先頭行からヘッダーを自動生成して Slack メッセージを送信します。
func (s *Client) SendText(ctx context.Context, message string) error {
	return s.SendTextWithHeader(ctx, generateHeader(message), message)
}

// buildWebhookMessage は Slack Incoming Webhook に送信するペイロードを構築します。
func (s *Client) buildWebhookMessage(headerText string, message string) (slack.WebhookMessage, error) {
	blocks, err := buildMessageBlocks(headerText, message)
	if err != nil {
		return slack.WebhookMessage{}, fmt.Errorf("slack Block Kitの構築に失敗しました: %w", err)
	}

	return slack.WebhookMessage{
		Text:      headerText,
		Username:  s.username,
		IconEmoji: s.iconEmoji,
		Channel:   s.channel,
		Blocks: &slack.Blocks{
			BlockSet: blocks,
		},
	}, nil
}

// generateHeader は本文から Slack メッセージのヘッダーを生成します。
func generateHeader(message string) string {
	if message == "" {
		return defaultHeader
	}

	firstLine := strings.SplitN(message, "\n", 2)[0]
	trimmedLine := strings.TrimSpace(firstLine)
	if trimmedLine == "" {
		return defaultHeader
	}

	return headerPrefix + truncateWithSuffixLimit(trimmedLine, maxHeaderLength, truncationEllipsis)
}

// truncateWithSuffixLimit は文字列を指定文字数以内に収め、必要に応じてサフィックスを付与します。
func truncateWithSuffixLimit(s string, maxLen int, suffix string) string {
	suffixLen := utf8.RuneCountInString(suffix)
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	if maxLen <= suffixLen {
		return text.Truncate(s, maxLen, "")
	}

	return text.Truncate(s, maxLen-suffixLen, suffix)
}
