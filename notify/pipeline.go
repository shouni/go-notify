package notify

import (
	"context"
	"errors"
)

const (
	// errorLabel は失敗通知でエラー内容に付けるラベルです。
	errorLabel = "エラー内容"
	// reasonLabel はスキップ通知で理由に付けるラベルです。
	reasonLabel = "理由"
)

// Titles はパイプライン通知の結果ごとの見出しです。
type Titles struct {
	// Success は正常終了時の見出しです。
	Success string
	// Failure は失敗時の見出しです。
	Failure string
	// Skipped は処理をスキップした場合の見出しです。
	Skipped string
}

// Pipeline は、非同期処理の成功・失敗・スキップを定型の見出しで通知します。
//
// 結果によって見出しが変わるだけで本文の組み立て方は共通、という
// パイプライン系サービスの通知形をそのまま表現しています。
// 見出しを実行時の条件で切り替えたい場合は WithTitles を使ってください。
type Pipeline struct {
	notifier Notifier
	titles   Titles
}

// NewPipeline はパイプライン通知を生成します。
// notifier が nil の場合は通知を行わない Notifier を使用します。
func NewPipeline(notifier Notifier, titles Titles) *Pipeline {
	if notifier == nil {
		notifier = Disabled()
	}
	return &Pipeline{notifier: notifier, titles: titles}
}

// WithTitles は見出しだけを差し替えた Pipeline を返します。元の Pipeline は変更しません。
//
// 見出しを実行時の条件で決めたい場合に使います（コマンドの種別ごとに完了通知の
// 文言を変える、など）。titles は元の見出しとマージせず、そのまま置き換えます。
// 使う結果の見出しだけ埋めれば十分です。
//
//	p.WithTitles(Titles{Success: titleFor(cmd)}).Success(ctx, body)
//
// これが無いと、見出しを切り替えたい呼び出し側は Pipeline を捨てて Notifier を
// 直接使うことになり、Enabled の判定と失敗時のエラー節の組み立てを各自で
// 再実装することになります。
func (p *Pipeline) WithTitles(titles Titles) *Pipeline {
	return &Pipeline{notifier: p.notifier, titles: titles}
}

// Enabled は通知が有効かどうかを返します。
// 本文の組み立てに費用がかかる場合、これで事前に打ち切れます。
func (p *Pipeline) Enabled() bool { return Enabled(p.notifier) }

// Success は正常終了を通知します。
func (p *Pipeline) Success(ctx context.Context, body *Body) error {
	return p.send(ctx, p.titles.Success, body)
}

// Failure は失敗を、原因とともに通知します。
// cause は本文の末尾に「エラー内容」として追記されます。
func (p *Pipeline) Failure(ctx context.Context, body *Body, cause error) error {
	body = orNewBody(body)
	body.Error(errorLabel, cause)
	return p.send(ctx, p.titles.Failure, body)
}

// Skipped は処理をスキップしたことを通知します。
// reason が非 nil の場合のみ、本文の末尾に「理由」として追記されます。
//
// Failure と違い nil を N/A で埋めないのは、スキップの理由が任意だからです。
// スキップは見出しだけで意味が通ることが多く（「差分がないためスキップしました」など）、
// そこに「理由: N/A」を足しても情報が増えません。一方 Failure の原因が nil なのは
// 想定外の状態なので、行を消さず N/A として残します。
func (p *Pipeline) Skipped(ctx context.Context, body *Body, reason error) error {
	body = orNewBody(body)
	if reason != nil {
		body.Error(reasonLabel, reason)
	}
	return p.send(ctx, p.titles.Skipped, body)
}

// send は見出しと本文を Message にまとめて送信します。
func (p *Pipeline) send(ctx context.Context, title string, body *Body) error {
	if title == "" {
		return errors.New("通知の見出しが設定されていません")
	}
	return p.notifier.Notify(ctx, Message{
		Title: title,
		Body:  orNewBody(body).String(),
	})
}

// orNewBody は nil の Body を空の Body に置き換えます。
func orNewBody(b *Body) *Body {
	if b == nil {
		return NewBody()
	}
	return b
}
