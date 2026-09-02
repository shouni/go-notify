// Package notify は、チャネル非依存の通知インターフェースと、
// パイプライン処理の結果を定型フォーマットで通知するための共通レイヤーを提供します。
//
// 実際の送信は各チャネルのサブパッケージ（slack など）が担当し、
// 本パッケージは「何を通知するか」だけを扱います。
package notify

import "context"

// NotAvailable は値が存在しない場合に表示する既定の文字列です。
const NotAvailable = "N/A"

// Level は通知が伝える結果の種別です。見出しの文言と違い機械が読める形で、
// チャネルが色やアイコンを出し分けるために使います。どう表現するかは各チャネルの判断です。
type Level int

const (
	// LevelNone は種別が未指定であることを表します。Level のゼロ値です。
	// チャネルは種別に依存しない既定の表示を選びます。
	LevelNone Level = iota
	// LevelSuccess は処理が正常に完了したことを表します。
	LevelSuccess
	// LevelFailure は処理が失敗したことを表します。
	LevelFailure
	// LevelSkipped は処理を実行しなかったことを表します。
	LevelSkipped
)

// String は Level の名前を返します。
//
// default に落とさず全種別を並べるのは、種別を足したときの追記漏れを exhaustive に
// 拾わせるためです。switch の後ろの return は定数以外の値（数値変換など）の受け皿です。
func (l Level) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelSuccess:
		return "success"
	case LevelFailure:
		return "failure"
	case LevelSkipped:
		return "skipped"
	}

	return "none"
}

// Message は 1 通の通知を表します。
type Message struct {
	// Title は通知の見出しです。空にはできません。
	Title string
	// Body は通知の本文です。組み立てには Body を使ってください。
	//
	// 記法は Body が出力する標準 Markdown です。Slack mrkdwn（*強調* や
	// <URL|表示テキスト>）を直接書くと、その本文は Slack 以外へ送れなくなります。
	Body string
	// Level は結果の種別です。ゼロ値（LevelNone）なら種別未指定として扱われます。
	//
	// Pipeline を使う場合は結果に応じて自動で設定されるため、
	// 呼び出し側が指定する必要はありません。
	Level Level
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
type disabledNotifier struct{}

// Notify は何もせずに nil を返します。
func (disabledNotifier) Notify(_ context.Context, _ Message) error { return nil }

// Disabled は通知を行わない Notifier を返します。通知はアプリケーションの主目的ではなく、
// 宛先が未設定であることはエラーではないため、送信を試みずに常に成功します。
func Disabled() Notifier { return disabledNotifier{} }

// Enabled は n が実際に送信を行う Notifier かどうかを返します（nil と Disabled は false）。
// 本文の組み立てに費用がかかる場合、これで事前に打ち切れます。
func Enabled(n Notifier) bool {
	if n == nil {
		return false
	}
	_, disabled := n.(disabledNotifier)
	return !disabled
}
