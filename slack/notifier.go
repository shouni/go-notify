package slack

import (
	"context"
	"errors"
	"strings"

	"github.com/shouni/go-http-kit/httpkit"
	"github.com/shouni/go-notify"
)

// Client が notify.Notifier を満たすことを保証します。
var _ notify.Notifier = (*Client)(nil)

// Notify は notify.Notifier インターフェースを実装します。
// msg.Title が空の場合は本文の先頭行から見出しを生成します。
func (s *Client) Notify(ctx context.Context, msg notify.Message) error {
	if msg.Title == "" {
		return s.SendText(ctx, msg.Body)
	}
	return s.SendTextWithHeader(ctx, msg.Title, msg.Body)
}

// NewNotifier は Slack Incoming Webhook への notify.Notifier を生成します。
//
// webhookURL が空文字または空白のみの場合は、Slack 通知が設定されていない
// ものとして notify.Disabled() を返します。通知はアプリケーションの主目的では
// なく、宛先の未設定は起動を妨げる理由にならないためです。
// 逆に webhookURL が設定されているのに client が nil の場合は、送信できない
// 設定ミスなのでエラーを返します。
func NewNotifier(client httpkit.Requester, webhookURL string, opts ...Option) (notify.Notifier, error) {
	if strings.TrimSpace(webhookURL) == "" {
		return notify.Disabled(), nil
	}
	if client == nil {
		return nil, errors.New("slack Webhook URLが設定されていますが、HTTPクライアントがnilです")
	}

	return NewClient(client, webhookURL, opts...)
}
