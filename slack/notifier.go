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
// good / danger / warning は Slack の組み込みキーワードで、空文字は「色を付けない」です。
//
// LevelNone にも値を持たせて全種別を並べるのは、種別を足したときの指定漏れを exhaustive に
// 拾わせるためです。抜けてもマップ参照は空文字を返して素通りし、無色で出てしまいます。
var levelColors = map[notify.Level]string{
	notify.LevelNone:    "",
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
	client     httpkit.Poster
	webhookURL string
}

// notifier が notify.Notifier を満たすことを保証します。
var _ notify.Notifier = (*notifier)(nil)

// NewNotifier は Slack Incoming Webhook への notify.Notifier を生成します。
//
// webhookURL が空なら通知未設定として notify.Disabled() を返します。設定されているのに
// client が nil の場合は、送信できない設定ミスなのでエラーを返します。
//
// # リトライについて
//
// Webhook への投稿は非冪等で、Slack に届いたのにレスポンスを取りこぼすと同じ通知が
// 二重に投稿されます。既定の httpkit.New はリトライが有効なので、クライアントを他の
// 用途と共有していると起こり得ます。取りこぼしと重複のどちらを避けたいかはアプリケーション
// 側の判断なので、本パッケージは方針を持たず渡された client をそのまま使います。
//
//	notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), webhookURL)
func NewNotifier(client httpkit.Poster, webhookURL string) (notify.Notifier, error) {
	if strings.TrimSpace(webhookURL) == "" {
		return notify.Disabled(), nil
	}
	if client == nil {
		return nil, errors.New("slack Webhook URLが設定されていますが、HTTPクライアントがnilです")
	}

	return &notifier{client: client, webhookURL: webhookURL}, nil
}

// Notify は notify.Notifier インターフェースを実装します。msg.Title は必須です。
// 本文から見出しを推測しないのは、何を見出しにするかが通知の意味を決める判断であり、
// 本文の 1 行目がそれである保証がないためです。
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

// redactedWebhookURL は、エラー文の中で Webhook URL の代わりに出す文字列です。
const redactedWebhookURL = "<webhook URL>"

// sendError は送信の失敗を、Webhook URL を伏せた文面で表します。
//
// Incoming Webhook の URL はそれ自体が投稿の認可なので、秘密として扱います。しかし
// HTTP クライアントは失敗の文面に宛先を含めます（net/http の *url.Error と、httpkit の
// 「HTTPリクエスト失敗 (URL: …)」の両方）。呼び出し側はこのエラーをそのままログに
// 出すので、ここで伏せておかないと秘密がログ基盤に残ります。
//
// Unwrap は元のエラーを返すため、errors.Is / errors.As による原因の判定はそのまま効きます。
// 伏せるのは Error() の文面だけです。
type sendError struct {
	webhookURL string
	cause      error
}

func (e *sendError) Error() string {
	return "slack Webhookメッセージの送信に失敗しました: " +
		strings.ReplaceAll(e.cause.Error(), e.webhookURL, redactedWebhookURL)
}

func (e *sendError) Unwrap() error { return e.cause }

// buildWebhookMessage は Slack Incoming Webhook に送信するペイロードを構築します。
//
// 色が対応する種別だけ attachment に包みます。attachment は左端に色帯が付く代わりに本文が
// 内側へ寄るため、種別が未指定の通知の見た目を変えないよう LevelNone は blocks のままです。
func (n *notifier) buildWebhookMessage(ctx context.Context, msg notify.Message) (slack.WebhookMessage, error) {
	blocks, err := buildMessageBlocks(ctx, msg.Title, msg.Body)
	if err != nil {
		return slack.WebhookMessage{}, fmt.Errorf("slack Block Kitの構築に失敗しました: %w", err)
	}

	var payload slack.WebhookMessage

	// 表に無い種別（数値からの変換など）は空文字になり、LevelNone と同じ扱いになります。
	color := levelColors[msg.Level]
	if color == "" {
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
