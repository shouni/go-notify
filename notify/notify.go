// Package notify は、チャネル非依存の通知インターフェースと、
// パイプライン処理の結果を定型フォーマットで通知するための共通レイヤーを提供します。
//
// 実際の送信は各チャネルのサブパッケージ（slack など）が担当し、
// 本パッケージは「何を通知するか」だけを扱います。
package notify

import "context"

// NotAvailable は値が存在しない場合に表示する既定の文字列です。
const NotAvailable = "N/A"

// Message は 1 通の通知を表します。
// Title が空の場合、送信側は Body から見出しを補完して構いません。
type Message struct {
	// Title は通知の見出しです。
	Title string
	// Body は通知の本文です。
	//
	// 記法はチャネル非依存の標準 Markdown です（太字は **強調**、
	// リンクは [表示テキスト](URL)）。各チャネルの方言への変換は送信側の
	// 責務なので、ここに Slack mrkdwn（*強調* や <URL|表示テキスト>）を
	// 直接書かないでください。書いた時点で、その本文は Slack 以外の
	// チャネルへ送れなくなります。
	//
	// 本文の組み立てには Body を使ってください。
	Body string
}

// Notifier は Message を送信先へ投稿します。
type Notifier interface {
	// Notify は msg を送信します。
	Notify(ctx context.Context, msg Message) error
}

// NotifierFunc は関数を Notifier として扱うためのアダプターです。
type NotifierFunc func(ctx context.Context, msg Message) error

// Notify は Notifier インターフェースを満たします。
func (f NotifierFunc) Notify(ctx context.Context, msg Message) error {
	return f(ctx, msg)
}

// disabledNotifier は何も送信しない Notifier です。
// 通知先が設定されていない場合に使用し、呼び出し側にエラーを返しません。
type disabledNotifier struct{}

// Notify は何もせずに nil を返します。
func (disabledNotifier) Notify(_ context.Context, _ Message) error { return nil }

// Disabled は通知を行わない Notifier を返します。
//
// 通知はアプリケーションの主目的ではなく、宛先が未設定であることは
// エラーではないため、送信を試みずに常に成功します。
func Disabled() Notifier { return disabledNotifier{} }

// Enabled は n が実際に送信を行う Notifier かどうかを返します。
// nil および Disabled が返した Notifier に対しては false を返します。
//
// 本文の組み立てに費用がかかる場合、これで事前に打ち切れます。
func Enabled(n Notifier) bool {
	if n == nil {
		return false
	}
	_, disabled := n.(disabledNotifier)
	return !disabled
}
