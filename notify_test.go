package notify_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shouni/go-notify"
)

// recorder は送信された Message を記録する Notifier です。
type recorder struct {
	got  []notify.Message
	fail error
}

// Notify は Message を記録し、fail が設定されていればそれを返します。
func (r *recorder) Notify(_ context.Context, msg notify.Message) error {
	r.got = append(r.got, msg)
	return r.fail
}

// TestDisabledNotifierSendsNothing は、無効な Notifier が送信も失敗もしないことを検証します。
func TestDisabledNotifierSendsNothing(t *testing.T) {
	if err := notify.Disabled().Notify(context.Background(), notify.Message{Title: "件名"}); err != nil {
		t.Errorf("Notify() = %v, want nil", err)
	}
}

// TestEnabled は Enabled の判定を検証します。
func TestEnabled(t *testing.T) {
	tests := []struct {
		name     string
		notifier notify.Notifier
		want     bool
	}{
		{name: "nil は無効", notifier: nil, want: false},
		{name: "Disabled は無効", notifier: notify.Disabled(), want: false},
		{name: "実装は有効", notifier: &recorder{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.Enabled(tt.notifier); got != tt.want {
				t.Errorf("Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNotifierFunc は関数アダプターが Notifier として機能することを検証します。
func TestNotifierFunc(t *testing.T) {
	var got notify.Message
	var fn notify.Notifier = notify.NotifierFunc(func(_ context.Context, msg notify.Message) error {
		got = msg
		return nil
	})

	if err := fn.Notify(context.Background(), notify.Message{Title: "件名", Body: "本文"}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Title != "件名" || got.Body != "本文" {
		t.Errorf("受け取った Message = %+v", got)
	}
}

// TestNotifierFuncPropagatesError は関数アダプターがエラーを伝えることを検証します。
func TestNotifierFuncPropagatesError(t *testing.T) {
	wantErr := errors.New("送信失敗")
	fn := notify.NotifierFunc(func(_ context.Context, _ notify.Message) error { return wantErr })

	if err := fn.Notify(context.Background(), notify.Message{}); !errors.Is(err, wantErr) {
		t.Errorf("Notify() = %v, want %v", err, wantErr)
	}
}
