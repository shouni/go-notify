package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shouni/go-http-kit/httpkit"
	"github.com/shouni/go-notify/notify"
	"github.com/slack-go/slack"
)

// levelColors は結果の種別を Slack の attachment の色に対応させます。
// good / danger / warning は Slack 側の組み込みキーワードです。
//
// LevelNone は意図的に含めません。種別が未指定なら色を付けず、
// トップレベルの blocks として投稿します。
var levelColors = map[notify.Level]string{
	notify.LevelSuccess: "good",
	notify.LevelFailure: "danger",
	notify.LevelSkipped: "warning",
}

// notifier は Slack Incoming Webhook へ通知を投稿します。
//
// 投稿者の表示名・アイコン・投稿先チャンネルは指定しません。Slack アプリ経由で
// 作った Incoming Webhook はこれらの上書きを受け付けず、常にアプリの設定を
// 継承するためです（受け付けるのは Slack が非推奨とした custom integration 版だけ）。
// サービスごとに表示を変えたい場合は、Slack アプリを分けてください。
type notifier struct {
	client     httpkit.Requester
	webhookURL string
}

// notifier が notify.Notifier を満たすことを保証します。
var _ notify.Notifier = (*notifier)(nil)

// NewNotifier は Slack Incoming Webhook への notify.Notifier を生成します。
//
// webhookURL が空文字または空白のみの場合は、Slack 通知が設定されていない
// ものとして notify.Disabled() を返します（理由は notify.Disabled を参照）。
// 逆に webhookURL が設定されているのに client が nil の場合は、送信できない
// 設定ミスなのでエラーを返します。
//
// # リトライについて
//
// Webhook への投稿は非冪等です。Slack には届いたのにレスポンスを取りこぼすと、
// リトライで同じ通知が二重に投稿されます。既定の httpkit.New(timeout) は
// リトライが有効なので、他の用途とクライアントを共有していると起こり得ます。
//
// 本パッケージはリトライ方針を持たず、渡された client をそのまま使います。
// 重複を避けたい場合は、タイムアウトや SSRF 対策の設定とコネクションプールを
// 共有したまま、リトライだけ切ったクライアントを渡してください。
//
//	notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), webhookURL)
//
// 取りこぼしと重複のどちらを避けたいかはアプリケーション側の判断なので、
// ライブラリでは決めません。
func NewNotifier(client httpkit.Requester, webhookURL string) (notify.Notifier, error) {
	if strings.TrimSpace(webhookURL) == "" {
		return notify.Disabled(), nil
	}
	if client == nil {
		return nil, errors.New("slack Webhook URLが設定されていますが、HTTPクライアントがnilです")
	}

	return &notifier{client: client, webhookURL: webhookURL}, nil
}

// Notify は notify.Notifier インターフェースを実装します。
//
// msg.Title は必須です。空の場合はエラーを返します。本文から見出しを
// 推測することはしません。何を見出しにするかは通知の意味を決める判断であり、
// 本文の 1 行目がそれである保証はどこにもないためです。
func (n *notifier) Notify(ctx context.Context, msg notify.Message) error {
	payload, err := n.buildWebhookMessage(ctx, msg)
	if err != nil {
		return err
	}

	if _, err := n.client.PostJSONAndFetchBytes(ctx, n.webhookURL, payload); err != nil {
		return fmt.Errorf("slack Webhookメッセージの送信に失敗しました: %w", err)
	}

	return nil
}

// buildWebhookMessage は Slack Incoming Webhook に送信するペイロードを構築します。
//
// 種別に色が対応する場合だけ attachment に包みます。attachment は左端に色帯が付く
// 代わりに本文が少し内側に寄るため、種別が未指定の通知まで見た目を変えないよう、
// LevelNone はトップレベルの blocks のままにします。
func (n *notifier) buildWebhookMessage(ctx context.Context, msg notify.Message) (slack.WebhookMessage, error) {
	blocks, err := buildMessageBlocks(ctx, msg.Title, msg.Body)
	if err != nil {
		return slack.WebhookMessage{}, fmt.Errorf("slack Block Kitの構築に失敗しました: %w", err)
	}

	var payload slack.WebhookMessage

	color, colored := levelColors[msg.Level]
	if !colored {
		// トップレベルの blocks がある場合、Text は本文として描画されず
		// プッシュ通知などのフォールバックにだけ使われます。
		payload.Text = msg.Title
		payload.Blocks = &slack.Blocks{BlockSet: blocks}
		return payload, nil
	}

	// attachment に包む場合は Text を空にします。blocks が attachment 側にあると
	// Text はフォールバックではなくメッセージ本文として描画されるため、
	// attachment 内の見出しブロックと合わせて見出しが 2 回出ます。
	// フォールバックの役目は Attachment.Fallback が引き継ぐので、
	// プッシュ通知の文言は失われません。
	payload.Attachments = []slack.Attachment{{
		Color:    color,
		Fallback: msg.Title,
		Blocks:   slack.Blocks{BlockSet: blocks},
	}}
	return payload, nil
}
