package notify_test

import (
	"errors"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// TestBodyWriters は各ライターの出力形式を検証します。
func TestBodyWriters(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "Field はラベルを太字にする",
			build: func(b *notify.Body) { b.Field("Title", "夏の終わり") },
			want:  "**Title:** 夏の終わり",
		},
		{
			name:  "Code は値を等幅にする",
			build: func(b *notify.Body) { b.Code("Command", "compose_video") },
			want:  "**Command:** `compose_video`",
		},
		{
			name:  "Link は表示テキスト付きリンクにする",
			build: func(b *notify.Body) { b.Link("History", "https://example.com/h/1", "job-1") },
			want:  "**History:** [job-1](https://example.com/h/1)",
		},
		{
			name:  "Link は表示テキスト省略時にURLを表示する",
			build: func(b *notify.Body) { b.Link("History", "https://example.com/h/1", "") },
			want:  "**History:** [https://example.com/h/1](https://example.com/h/1)",
		},
		{
			name:  "Text は素の行を書き込む",
			build: func(b *notify.Body) { b.Text("そのままの行") },
			want:  "そのままの行",
		},
		{
			name: "複数行は改行で連結される",
			build: func(b *notify.Body) {
				b.Code("Command", "compose").Field("Title", "曲名")
			},
			want: "**Command:** `compose`\n**Title:** 曲名",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := notify.NewBody()
			tt.build(b)
			if got := b.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBodySkipsEmptyValues は、値が空の項目が出力されないことを検証します。
// 呼び出し側から項目ごとの存在チェックを取り除くための中心的な性質です。
func TestBodySkipsEmptyValues(t *testing.T) {
	b := notify.NewBody()
	b.Field("Title", "")
	b.Code("Command", "")
	b.Link("History", "", "ラベルだけあってもURLが無い")
	b.Text("")

	if !b.Empty() {
		t.Fatalf("空の値だけを書き込んだのに Empty() = false, 本文 = %q", b.String())
	}
}

// TestBodyStringFallsBackToNotAvailable は、何も書き込まれていない本文が
// NotAvailable になることを検証します。
func TestBodyStringFallsBackToNotAvailable(t *testing.T) {
	if got := notify.NewBody().String(); got != notify.NotAvailable {
		t.Errorf("String() = %q, want %q", got, notify.NotAvailable)
	}
}

// TestBodyZeroValueIsUsable は、ゼロ値の Body がそのまま使えることを検証します。
func TestBodyZeroValueIsUsable(t *testing.T) {
	var b notify.Body
	b.Field("Title", "ゼロ値")

	if got, want := b.String(), "**Title:** ゼロ値"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyError はエラー節の書式とフォールバックを検証します。
func TestBodyError(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "err が nil の場合は N/A",
			build: func(b *notify.Body) { b.Error("エラー内容", nil) },
			want:  "**エラー内容:**\n" + notify.NotAvailable,
		},
		{
			name:  "err の内容を表示する",
			build: func(b *notify.Body) { b.Error("エラー内容", errors.New("失敗しました")) },
			want:  "**エラー内容:**\n失敗しました",
		},
		{
			name: "本文がある場合は空行で区切る",
			build: func(b *notify.Body) {
				b.Code("Command", "compose").Error("エラー内容", errors.New("失敗しました"))
			},
			want: "**Command:** `compose`\n\n**エラー内容:**\n失敗しました",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := notify.NewBody()
			tt.build(b)
			if got := b.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBodyBlockSanitizesBacktick は、コードブロックがバックティックで
// 途中終了しないことを検証します。
func TestBodyBlockSanitizesBacktick(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "exec `gsutil` failed")

	want := "**エラー詳細:**\n```exec 'gsutil' failed```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyCodeSanitizesBacktick は、等幅表示がバックティックで壊れないことを検証します。
func TestBodyCodeSanitizesBacktick(t *testing.T) {
	b := notify.NewBody()
	b.Code("Command", "ls `pwd`")

	want := "**Command:** `ls 'pwd'`"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyBlockFallsBackToNotAvailable は、内容が空のブロックの表示を検証します。
func TestBodyBlockFallsBackToNotAvailable(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "")

	want := "**エラー詳細:**\n```" + notify.NotAvailable + "```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
