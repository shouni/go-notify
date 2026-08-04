package notify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shouni/go-notify"
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

			if err := tt.call(p, notify.NewBody().Code("Command", "compose")); err != nil {
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

// TestPipelineFailureAppendsCause は、失敗通知が本文にエラー内容を追記することを検証します。
func TestPipelineFailureAppendsCause(t *testing.T) {
	rec := &recorder{}
	p := notify.NewPipeline(rec, testTitles)

	body := notify.NewBody().Code("Job ID", "job-1")
	if err := p.Failure(context.Background(), body, errors.New("Veo がタイムアウトしました")); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	want := "**Job ID:** `job-1`\n\n**エラー内容:**\nVeo がタイムアウトしました"
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

	body := notify.NewBody().Field("Title", "曲名")
	if err := p.Failure(context.Background(), body, errors.New("原因")); err != nil {
		t.Fatalf("通知に失敗しました: %v", err)
	}

	if !strings.Contains(body.String(), "**エラー内容:**") {
		t.Errorf("渡した Body にエラー内容が追記されていません: %q", body.String())
	}
}
