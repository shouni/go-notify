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
// 結果で見出しだけが変わり本文の組み立ては共通、というパイプライン系の形をそのまま
// 表現しています。実行時に見出しを切り替えたい場合は WithTitles を使ってください。
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

// WithTitles は見出しだけを差し替えた Pipeline を返します。元は変更せず、titles は
// マージせずに置き換えるので、これから送る結果の見出しだけ埋めれば十分です。
//
//	p.WithTitles(Titles{Success: titleFor(cmd)}).Success(ctx, body)
//
// これが無いと、見出しを切り替えたい側は Pipeline を捨てて Notifier を直接使うことになり、
// Enabled の判定と失敗時のエラー節を各自で再実装することになります。
func (p *Pipeline) WithTitles(titles Titles) *Pipeline {
	return &Pipeline{notifier: p.notifier, titles: titles}
}

// Enabled は通知が有効かどうかを返します。判定は関数の Enabled と同じです。
func (p *Pipeline) Enabled() bool { return Enabled(p.notifier) }

// Success は正常終了を通知します。
func (p *Pipeline) Success(ctx context.Context, body *Body) error {
	return p.send(ctx, LevelSuccess, p.titles.Success, body)
}

// Failure は失敗を、原因とともに通知します。cause は本文の末尾に「エラー内容」として
// 追記されます。追記先はコピーなので、渡した body は変更されません（理由は derive）。
func (p *Pipeline) Failure(ctx context.Context, body *Body, cause error) error {
	body = derive(body)
	body.Error(errorLabel, cause)
	return p.send(ctx, LevelFailure, p.titles.Failure, body)
}

// Skipped は処理をスキップしたことを通知します。reason が非 nil のときだけ、本文の
// 末尾に「理由」として追記されます。追記先はコピーです（理由は derive）。
//
// Failure と違い nil を N/A で埋めないのは、スキップの理由が任意だからです。見出しだけで
// 意味が通ることが多く、「理由: N/A」を足しても情報が増えません。Failure の原因が nil な
// のは想定外の状態なので、あちらは行を残します。
func (p *Pipeline) Skipped(ctx context.Context, body *Body, reason error) error {
	body = derive(body)
	if reason != nil {
		body.Error(reasonLabel, reason)
	}
	return p.send(ctx, LevelSkipped, p.titles.Skipped, body)
}

// send は種別・見出し・本文を Message にまとめて送信します。種別をここで確定するのは、
// 呼び出し側に指定させると見出しと種別が食い違う余地を作るだけだからです。
func (p *Pipeline) send(ctx context.Context, level Level, title string, body *Body) error {
	if title == "" {
		return errors.New("通知の見出しが設定されていません")
	}
	return p.notifier.Notify(ctx, Message{
		Title: title,
		Body:  orNewBody(body).String(),
		Level: level,
	})
}

// orNewBody は nil の Body を空の Body に置き換えます。
func orNewBody(b *Body) *Body {
	if b == nil {
		return NewBody()
	}
	return b
}

// derive は b と同じ内容を持つ別の Body を返します。b が nil なら空の Body です。
//
// Failure と Skipped の追記を渡された Body に直接書くと、同じ Body を使い回した次の通知に
// 前回の節が残ります（成功→失敗→スキップで、スキップ通知に前の「エラー内容」が載る）。
// 結果ごとに Body を渡し直せる API である以上、使い回しは想定内なのでコピー側に追記します。
func derive(b *Body) *Body {
	return orNewBody(b).clone()
}
