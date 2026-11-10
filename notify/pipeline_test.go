package notify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// testTitles is the set of headings used in tests.
var testTitles = notify.Titles{
	Success: "✅ 完了しました",
	Failure: "❌ 失敗しました",
	Skipped: "⏭️ スキップしました",
}

// TestPipelineTitles verifies that the right heading is chosen for each
// outcome.
func TestPipelineTitles(t *testing.T) {
	tests := []struct {
		name string
		call func(p *notify.Pipeline, b *notify.Body) error
		want string
	}{
		{
			name: "Success",
			call: func(p *notify.Pipeline, b *notify.Body) error {
				return p.Success(context.Background(), b)
			},
			want: testTitles.Success,
		},
		{
			name: "Failure",
			call: func(p *notify.Pipeline, b *notify.Body) error {
				return p.Failure(context.Background(), b, errors.New("失敗"))
			},
			want: testTitles.Failure,
		},
		{
			name: "Skipped",
			call: func(p *notify.Pipeline, b *notify.Body) error {
				return p.Skipped(context.Background(), b, errors.New("差分なし"))
			},
			want: testTitles.Skipped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			p := notify.NewPipeline(rec, testTitles)

			if err := tt.call(p, notify.NewBody().Code("Command", "run_task")); err != nil {
				t.Fatalf("notify failed: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("sent count = %d, want 1", len(rec.got))
			}
			if rec.got[0].Title != tt.want {
				t.Errorf("Title = %q, want %q", rec.got[0].Title, tt.want)
			}
		})
	}
}

// TestPipelineSetsLevel verifies that the level for each outcome is set
// automatically. Callers never set Level, so the heading and the level
// cannot disagree.
func TestPipelineSetsLevel(t *testing.T) {
	tests := []struct {
		name string
		call func(p *notify.Pipeline) error
		want notify.Level
	}{
		{
			name: "Success",
			call: func(p *notify.Pipeline) error { return p.Success(context.Background(), nil) },
			want: notify.LevelSuccess,
		},
		{
			name: "Failure",
			call: func(p *notify.Pipeline) error { return p.Failure(context.Background(), nil, errors.New("原因")) },
			want: notify.LevelFailure,
		},
		{
			name: "Skipped",
			call: func(p *notify.Pipeline) error { return p.Skipped(context.Background(), nil, nil) },
			want: notify.LevelSkipped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			if err := tt.call(notify.NewPipeline(rec, testTitles)); err != nil {
				t.Fatalf("notify failed: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("sent count = %d, want 1", len(rec.got))
			}
			if got := rec.got[0].Level; got != tt.want {
				t.Errorf("Level = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMessageLevelZeroValueIsNone verifies that the zero value of Level
// means none was given. Existing callers that build a Message by hand rely
// on it to keep their behaviour.
func TestMessageLevelZeroValueIsNone(t *testing.T) {
	var msg notify.Message
	if msg.Level != notify.LevelNone {
		t.Errorf("zero-value Level = %v, want %v", msg.Level, notify.LevelNone)
	}
}

// TestLevelString verifies the names of the levels.
func TestLevelString(t *testing.T) {
	tests := []struct {
		level notify.Level
		want  string
	}{
		{notify.LevelNone, "none"},
		{notify.LevelSuccess, "success"},
		{notify.LevelFailure, "failure"},
		{notify.LevelSkipped, "skipped"},
		{notify.Level(99), "none"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPipelineFailureAppendsCause verifies that a failure notification
// appends the error to the body.
func TestPipelineFailureAppendsCause(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	body := notify.NewBody().Code("Job ID", "job-1")
	if err := p.Failure(context.Background(), body, errors.New("タイムアウトしました")); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	want := "**Job ID:** `job-1`\n\n**エラー内容:**\nタイムアウトしました"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineSkippedAppendsReason verifies that a skipped notification
// appends the reason to the body.
func TestPipelineSkippedAppendsReason(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Skipped(context.Background(), nil, errors.New("差分がありません")); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	want := "**理由:**\n差分がありません"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineSkippedOmitsNilReason verifies that a nil reason omits the
// whole 「理由」 section. A skip usually makes sense from the heading alone,
// and 「理由: N/A」 would add nothing.
func TestPipelineSkippedOmitsNilReason(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Skipped(context.Background(), notify.NewBody().Code("Job ID", "job-1"), nil); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	want := "**Job ID:** `job-1`"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineFailureKeepsNilCause verifies that, unlike Skipped, a nil
// cause still leaves the 「エラー内容」 section as N/A. A failure without a
// cause is an unexpected state and must not be hidden.
func TestPipelineFailureKeepsNilCause(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Failure(context.Background(), nil, nil); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	want := "**エラー内容:**\n" + notify.NotAvailable
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineNilBody verifies that sending works without a body.
func TestPipelineNilBody(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Success(context.Background(), nil); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	if rec.got[0].Body != notify.NotAvailable {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, notify.NotAvailable)
	}
}

// TestPipelineNilNotifier verifies that a missing Notifier only disables
// notifications and is not an error.
func TestPipelineNilNotifier(t *testing.T) {
	p := notify.NewPipeline(nil, testTitles)

	if p.Enabled() {
		t.Error("Enabled() = true, want false")
	}
	if err := p.Success(context.Background(), notify.NewBody().Text("本文")); err != nil {
		t.Errorf("Success() = %v, want nil", err)
	}
}

// TestPipelineEnabled verifies that Enabled reflects the Notifier.
func TestPipelineEnabled(t *testing.T) {
	if p := notify.NewPipeline(&recorder{}, testTitles); !p.Enabled() {
		t.Error("Enabled() with an implementation = false, want true")
	}
	if p := notify.NewPipeline(notify.Disabled(), testTitles); p.Enabled() {
		t.Error("Enabled() with Disabled = true, want false")
	}
}

// TestPipelineWithTitles verifies that the Pipeline with replaced headings
// is the one used.
func TestPipelineWithTitles(t *testing.T) {
	rec := &recorder{}
	base := notify.NewPipeline(rec, testTitles)

	err := base.WithTitles(notify.Titles{Success: "🎨 別種の処理が完了しました"}).
		Success(context.Background(), notify.NewBody().Field("Job ID", "job-1"))
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if len(rec.got) != 1 {
		t.Fatalf("sent count = %d, want 1", len(rec.got))
	}
	if got, want := rec.got[0].Title, "🎨 別種の処理が完了しました"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
}

// TestPipelineWithTitlesLeavesOriginalIntact verifies that WithTitles does
// not modify the original Pipeline. The intended use keeps one Pipeline and
// changes headings per call, so shared state must not be rewritten.
func TestPipelineWithTitlesLeavesOriginalIntact(t *testing.T) {
	rec := &recorder{}
	base := notify.NewPipeline(rec, testTitles)

	if err := base.WithTitles(notify.Titles{Success: "別の見出し"}).Success(context.Background(), nil); err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	if err := base.Success(context.Background(), nil); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if len(rec.got) != 2 {
		t.Fatalf("sent count = %d, want 2", len(rec.got))
	}
	if got := rec.got[1].Title; got != testTitles.Success {
		t.Errorf("original Pipeline's Title = %q, want %q", got, testTitles.Success)
	}
}

// TestPipelineWithTitlesKeepsNotifier verifies that whether notifications are
// enabled carries over after the headings are replaced.
func TestPipelineWithTitlesKeepsNotifier(t *testing.T) {
	if p := notify.NewPipeline(notify.Disabled(), testTitles); p.WithTitles(testTitles).Enabled() {
		t.Error("Enabled() derived from Disabled = true, want false")
	}
	if p := notify.NewPipeline(&recorder{}, testTitles); !p.WithTitles(testTitles).Enabled() {
		t.Error("Enabled() derived from an implementation = false, want true")
	}
}

// TestPipelineMissingTitle verifies that notifying an outcome with no
// heading returns an error instead of silently sending.
func TestPipelineMissingTitle(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, notify.Titles{Success: "✅ 完了"})

	err := p.Skipped(context.Background(), nil, errors.New("理由"))
	if err == nil {
		t.Fatal("Skipped() = nil, want error")
	}
	if len(rec.got) != 0 {
		t.Errorf("sent count = %d, want 0", len(rec.got))
	}
}

// TestPipelinePropagatesSendError verifies that a send error reaches the
// caller.
func TestPipelinePropagatesSendError(t *testing.T) {
	wantErr := errors.New("webhook 5xx")
	p := notify.NewPipeline(&recorder{fail: wantErr}, testTitles)

	if err := p.Success(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Errorf("Success() = %v, want %v", err, wantErr)
	}
}

// TestPipelineFailureKeepsCallerBodyIntact verifies that Failure does not
// rewrite the Body it was given. The append goes to a copy.
func TestPipelineFailureKeepsCallerBodyIntact(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	body := notify.NewBody().Field("Title", "サンプルタイトル")
	if err := p.Failure(context.Background(), body, errors.New("原因")); err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if strings.Contains(body.String(), "**エラー内容:**") {
		t.Errorf("the Body passed in was rewritten: %q", body.String())
	}
	if !strings.Contains(rec.got[0].Body, "**エラー内容:**") {
		t.Errorf("the sent body has no error section: %q", rec.got[0].Body)
	}
}

// TestPipelineBodyReuseAcrossOutcomes verifies that passing one Body again
// for each outcome does not carry over the previous append.
//
// This was the easiest trap to fall into in this package. While Failure
// wrote straight into the caller's Body, sending success → failure →
// skipped put the earlier 「エラー内容」 into the skipped notification's body.
func TestPipelineBodyReuseAcrossOutcomes(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)
	ctx := context.Background()

	body := notify.NewBody().Code("Job ID", "job-1")
	if err := p.Success(ctx, body); err != nil {
		t.Fatalf("success notification failed: %v", err)
	}
	if err := p.Failure(ctx, body, errors.New("原因")); err != nil {
		t.Fatalf("failure notification failed: %v", err)
	}
	if err := p.Skipped(ctx, body, errors.New("差分がありません")); err != nil {
		t.Fatalf("skipped notification failed: %v", err)
	}

	wants := []string{
		"**Job ID:** `job-1`",
		"**Job ID:** `job-1`\n\n**エラー内容:**\n原因",
		"**Job ID:** `job-1`\n\n**理由:**\n差分がありません",
	}
	if len(rec.got) != len(wants) {
		t.Fatalf("sent count = %d, want %d", len(rec.got), len(wants))
	}
	for i, want := range wants {
		if rec.got[i].Body != want {
			t.Errorf("Body of message %d = %q, want %q", i, rec.got[i].Body, want)
		}
	}
}
