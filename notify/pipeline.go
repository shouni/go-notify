package notify

import (
	"context"
	"errors"
)

const (
	// errorLabel labels the error text in a failure notification. It is
	// Japanese on purpose: it appears in Slack for the owner.
	errorLabel = "エラー内容"
	// reasonLabel labels the reason in a skipped notification (Japanese for
	// the same reason).
	reasonLabel = "理由"
)

// Titles holds a pipeline notification's heading for each outcome.
type Titles struct {
	// Success is the heading for a successful run.
	Success string
	// Failure is the heading for a failed run.
	Failure string
	// Skipped is the heading for a skipped run.
	Skipped string
}

// Pipeline notifies the success, failure or skipping of asynchronous work
// under fixed headings. It mirrors the shape of pipeline work: only the
// heading changes with the outcome, while the body is built the same way.
// Use WithTitles to change the headings at run time.
type Pipeline struct {
	notifier Notifier
	titles   Titles
}

// NewPipeline creates a pipeline notifier. A nil notifier is replaced with
// one that does not notify.
func NewPipeline(notifier Notifier, titles Titles) *Pipeline {
	if notifier == nil {
		notifier = Disabled()
	}
	return &Pipeline{notifier: notifier, titles: titles}
}

// WithTitles returns a Pipeline with only the headings replaced. The
// original is unchanged, and titles replaces rather than merges, so filling
// in the heading of the outcome about to be sent is enough.
//
//	p.WithTitles(Titles{Success: titleFor(cmd)}).Success(ctx, body)
//
// Without it, a caller that needs to switch headings abandons Pipeline for
// the Notifier and re-implements the Enabled check and the failure's error
// section on its own.
func (p *Pipeline) WithTitles(titles Titles) *Pipeline {
	return &Pipeline{notifier: p.notifier, titles: titles}
}

// Enabled reports whether notifications are enabled, by the same test as
// the Enabled function.
func (p *Pipeline) Enabled() bool { return Enabled(p.notifier) }

// Success notifies a successful run.
func (p *Pipeline) Success(ctx context.Context, body *Body) error {
	return p.send(ctx, LevelSuccess, p.titles.Success, body)
}

// Failure notifies a failure together with its cause. cause is appended to
// the end of the body under the 「エラー内容」 label. The append goes to a
// copy, so the body passed in is not modified (see derive for why).
func (p *Pipeline) Failure(ctx context.Context, body *Body, cause error) error {
	body = derive(body)
	body.Error(errorLabel, cause)
	return p.send(ctx, LevelFailure, p.titles.Failure, body)
}

// Skipped notifies that the work was skipped. Only a non-nil reason is
// appended to the end of the body, under the 「理由」 label. The append goes
// to a copy (see derive for why).
//
// Unlike Failure it does not fill a nil with N/A, because a reason for
// skipping is optional: the heading usually says enough, and 「理由: N/A」
// adds nothing. A nil cause for Failure is an unexpected state, so that one
// keeps the line.
func (p *Pipeline) Skipped(ctx context.Context, body *Body, reason error) error {
	body = derive(body)
	if reason != nil {
		body.Error(reasonLabel, reason)
	}
	return p.send(ctx, LevelSkipped, p.titles.Skipped, body)
}

// send packs the level, heading and body into a Message and sends it. The
// level is fixed here because letting callers set it only creates room for
// the heading and the level to disagree.
func (p *Pipeline) send(ctx context.Context, level Level, title string, body *Body) error {
	if title == "" {
		return errors.New("notification title is not set")
	}
	return p.notifier.Notify(ctx, Message{
		Title: title,
		Body:  orNewBody(body).String(),
		Level: level,
	})
}

// orNewBody replaces a nil Body with an empty one.
func orNewBody(b *Body) *Body {
	if b == nil {
		return NewBody()
	}
	return b
}

// derive returns a separate Body with the same content as b, or an empty
// Body when b is nil.
//
// If Failure and Skipped appended to the Body they were given, the next
// notification reusing that Body would carry the previous section (success
// → failure → skipped put the earlier 「エラー内容」 into the skipped
// notification). An API that takes a Body per outcome invites reuse, so the
// append goes to a copy.
func derive(b *Body) *Body {
	return orNewBody(b).clone()
}
