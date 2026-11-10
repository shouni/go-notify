// Package notify provides a channel-agnostic notification interface and a
// shared layer for reporting pipeline outcomes in a fixed format.
//
// Sending is done by each channel's subpackage (slack and so on); this
// package deals only with what to notify.
package notify

import "context"

// NotAvailable is the default text shown when a value is absent.
const NotAvailable = "N/A"

// Level is the kind of outcome a notification reports. Unlike the heading
// text it is machine-readable, so a channel can pick a colour or an icon
// from it. How to present it is up to each channel.
type Level int

const (
	// LevelNone means no level was given. It is the zero value of Level.
	// A channel chooses its default, level-independent presentation.
	LevelNone Level = iota
	// LevelSuccess means the work completed successfully.
	LevelSuccess
	// LevelFailure means the work failed.
	LevelFailure
	// LevelSkipped means the work was not run.
	LevelSkipped
)

// String returns the name of the Level.
//
// Every level is listed instead of falling into default so that exhaustive
// catches a level added without a case. The return after the switch handles
// values that are not one of the constants (a numeric conversion, say).
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

// Message is a single notification.
type Message struct {
	// Title is the notification heading. It must not be empty.
	Title string
	// Body is the notification body. Build it with Body.
	//
	// Its markup is the standard Markdown that Body emits. Writing Slack
	// mrkdwn (*bold* or <URL|text>) directly makes the body unsendable to
	// any channel other than Slack.
	Body string
	// Level is the kind of outcome. The zero value (LevelNone) means none
	// was given.
	//
	// Pipeline sets it from the outcome, so callers using Pipeline do not
	// need to set it.
	Level Level
}

// Notifier posts a Message to its destination.
type Notifier interface {
	// Notify sends msg.
	Notify(ctx context.Context, msg Message) error
}

// NotifierFunc is an adapter that lets a function be used as a Notifier.
type NotifierFunc func(ctx context.Context, msg Message) error

// Notify implements the Notifier interface.
func (f NotifierFunc) Notify(ctx context.Context, msg Message) error {
	return f(ctx, msg)
}

// disabledNotifier is a Notifier that sends nothing.
type disabledNotifier struct{}

// Notify does nothing and returns nil.
func (disabledNotifier) Notify(_ context.Context, _ Message) error { return nil }

// Disabled returns a Notifier that does not notify. Notifying is not an
// application's main purpose and an unset destination is not an error, so
// it always succeeds without attempting to send.
func Disabled() Notifier { return disabledNotifier{} }

// Enabled reports whether n is a Notifier that actually sends (false for nil
// and Disabled). Callers can use it to skip building an expensive body.
func Enabled(n Notifier) bool {
	if n == nil {
		return false
	}
	_, disabled := n.(disabledNotifier)
	return !disabled
}
