package notify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// testTitles はテスト用の見出しセットです。
var testTitles = notify.Titles{
	Success: "✅ 完了しました",
	Failure: "❌ 失敗しました",
	Skipped: "⏭️ スキップしました",
}

// TestPipelineTitles は結果ごとに正しい見出しが選ばれることを検証します。
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
				t.Fatalf("通知に失敗しました: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("送信件数 = %d, want 1", len(rec.got))
			}
			if rec.got[0].Title != tt.want {
				t.Errorf("Title = %q, want %q", rec.got[0].Title, tt.want)
			}
		})
	}
}

// TestPipelineSetsLevel は、結果ごとの種別が自動で設定されることを検証します。
// 呼び出し側は Level を指定しないので、見出しと種別が食い違う余地がありません。
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
				t.Fatalf("通知に失敗しました: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("送信件数 = %d, want 1", len(rec.got))
			}
			if got := rec.got[0].Level; got != tt.want {
				t.Errorf("Level = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMessageLevelZeroValueIsNone は、Level のゼロ値が未指定を表すことを検証します。
// Message を直接組み立てている既存の呼び出し側の挙動を変えないための前提です。
func TestMessageLevelZeroValueIsNone(t *testing.T) {
	var msg notify.Message
	if msg.Level != notify.LevelNone {
		t.Errorf("ゼロ値の Level = %v, want %v", msg.Level, notify.LevelNone)
	}
}

// TestLevelString は Level の名前を検証します。
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

// TestPipelineFailureAppendsCause は、失敗通知が本文にエラー内容を追記することを検証します。
func TestPipelineFailureAppendsCause(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	body := notify.NewBody().Code("Job ID", "job-1")
	if err := p.Failure(context.Background(), body, errors.New("タイムアウトしました")); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	want := "**Job ID:** `job-1`\n\n**エラー内容:**\nタイムアウトしました"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineSkippedAppendsReason は、スキップ通知が本文に理由を追記することを検証します。
func TestPipelineSkippedAppendsReason(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Skipped(context.Background(), nil, errors.New("差分がありません")); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	want := "**理由:**\n差分がありません"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineSkippedOmitsNilReason は、理由が nil の場合に「理由」節ごと
// 省かれることを検証します。スキップは見出しだけで意味が通ることが多く、
// 「理由: N/A」を足しても情報が増えないためです。
func TestPipelineSkippedOmitsNilReason(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Skipped(context.Background(), notify.NewBody().Code("Job ID", "job-1"), nil); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	want := "**Job ID:** `job-1`"
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineFailureKeepsNilCause は、Skipped と対照的に、原因が nil でも
// 「エラー内容」節が N/A として残ることを検証します。
// 失敗したのに原因が無いのは想定外の状態であり、消してはいけません。
func TestPipelineFailureKeepsNilCause(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Failure(context.Background(), nil, nil); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	want := "**エラー内容:**\n" + notify.NotAvailable
	if rec.got[0].Body != want {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, want)
	}
}

// TestPipelineNilBody は、本文を渡さなくても送信できることを検証します。
func TestPipelineNilBody(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	if err := p.Success(context.Background(), nil); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}
	if rec.got[0].Body != notify.NotAvailable {
		t.Errorf("Body = %q, want %q", rec.got[0].Body, notify.NotAvailable)
	}
}

// TestPipelineNilNotifier は、Notifier 未指定でも通知が無効化されるだけで
// エラーにならないことを検証します。
func TestPipelineNilNotifier(t *testing.T) {
	p := notify.NewPipeline(nil, testTitles)

	if p.Enabled() {
		t.Error("Enabled() = true, want false")
	}
	if err := p.Success(context.Background(), notify.NewBody().Text("本文")); err != nil {
		t.Errorf("Success() = %v, want nil", err)
	}
}

// TestPipelineEnabled は Enabled が Notifier の状態を反映することを検証します。
func TestPipelineEnabled(t *testing.T) {
	if p := notify.NewPipeline(&recorder{}, testTitles); !p.Enabled() {
		t.Error("実装を渡した場合の Enabled() = false, want true")
	}
	if p := notify.NewPipeline(notify.Disabled(), testTitles); p.Enabled() {
		t.Error("Disabled を渡した場合の Enabled() = true, want false")
	}
}

// TestPipelineWithTitles は、見出しを差し替えた Pipeline が使われることを検証します。
func TestPipelineWithTitles(t *testing.T) {
	rec := &recorder{}
	base := notify.NewPipeline(rec, testTitles)

	err := base.WithTitles(notify.Titles{Success: "🎨 別種の処理が完了しました"}).
		Success(context.Background(), notify.NewBody().Field("Job ID", "job-1"))
	if err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	if len(rec.got) != 1 {
		t.Fatalf("送信件数 = %d, want 1", len(rec.got))
	}
	if got, want := rec.got[0].Title, "🎨 別種の処理が完了しました"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
}

// TestPipelineWithTitlesLeavesOriginalIntact は、WithTitles が元の Pipeline を
// 変更しないことを検証します。1 つの Pipeline を保持したまま呼び出しごとに
// 見出しを変える使い方が前提なので、共有状態を書き換えてはいけません。
func TestPipelineWithTitlesLeavesOriginalIntact(t *testing.T) {
	rec := &recorder{}
	base := notify.NewPipeline(rec, testTitles)

	if err := base.WithTitles(notify.Titles{Success: "別の見出し"}).Success(context.Background(), nil); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}
	if err := base.Success(context.Background(), nil); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	if len(rec.got) != 2 {
		t.Fatalf("送信件数 = %d, want 2", len(rec.got))
	}
	if got := rec.got[1].Title; got != testTitles.Success {
		t.Errorf("元の Pipeline の Title = %q, want %q", got, testTitles.Success)
	}
}

// TestPipelineWithTitlesKeepsNotifier は、差し替え後も通知の有効・無効が
// 引き継がれることを検証します。
func TestPipelineWithTitlesKeepsNotifier(t *testing.T) {
	if p := notify.NewPipeline(notify.Disabled(), testTitles); p.WithTitles(testTitles).Enabled() {
		t.Error("Disabled から派生した Enabled() = true, want false")
	}
	if p := notify.NewPipeline(&recorder{}, testTitles); !p.WithTitles(testTitles).Enabled() {
		t.Error("実装から派生した Enabled() = false, want true")
	}
}

// TestPipelineMissingTitle は、見出し未設定の結果を通知しようとした場合に
// 黙って送信せずエラーを返すことを検証します。
func TestPipelineMissingTitle(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, notify.Titles{Success: "✅ 完了"})

	err := p.Skipped(context.Background(), nil, errors.New("理由"))
	if err == nil {
		t.Fatal("Skipped() = nil, want error")
	}
	if len(rec.got) != 0 {
		t.Errorf("送信件数 = %d, want 0", len(rec.got))
	}
}

// TestPipelinePropagatesSendError は、送信エラーが呼び出し元へ伝わることを検証します。
func TestPipelinePropagatesSendError(t *testing.T) {
	wantErr := errors.New("webhook 5xx")
	p := notify.NewPipeline(&recorder{fail: wantErr}, testTitles)

	if err := p.Success(context.Background(), nil); !errors.Is(err, wantErr) {
		t.Errorf("Success() = %v, want %v", err, wantErr)
	}
}

// TestPipelineFailureKeepsCallerBodyIntact は、Failure が呼び出し側の Body を
// 破壊的に書き換えても、同じ Body を使い回さない限り影響がないことを示します。
// 追記は意図した動作であり、この境界を明示しておきます。
func TestPipelineFailureAppendsToGivenBody(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	body := notify.NewBody().Field("Title", "サンプルタイトル")
	if err := p.Failure(context.Background(), body, errors.New("原因")); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	if !strings.Contains(body.String(), "**エラー内容:**") {
		t.Errorf("渡した Body にエラー内容が追記されていません: %q", body.String())
	}
}
