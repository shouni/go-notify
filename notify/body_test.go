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
			build: func(b *notify.Body) { b.Field("Title", "サンプルタイトル") },
			want:  "**Title:** サンプルタイトル",
		},
		{
			name:  "Code は値を等幅にする",
			build: func(b *notify.Body) { b.Code("Command", "run_task") },
			want:  "**Command:** `run_task`",
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
				b.Code("Command", "run_task").Field("Title", "サンプルタイトル")
			},
			want: "**Command:** `run_task`\n**Title:** サンプルタイトル",
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
				b.Code("Command", "run_task").Error("エラー内容", errors.New("失敗しました"))
			},
			want: "**Command:** `run_task`\n\n**エラー内容:**\n失敗しました",
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
	b.Block("エラー詳細", "exec `run_task` failed")

	want := "**エラー詳細:**\n```\nexec 'run_task' failed\n```"
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

	want := "**エラー詳細:**\n```\n" + notify.NotAvailable + "\n```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyBlockFencesAreOnTheirOwnLines は、フェンスが独立した行に置かれることを検証します。
//
// ```内容``` と 1 行に詰めると内容の 1 行目が言語指定として食われ、
// 終了フェンスも行頭に無いのでブロックが閉じません。
func TestBodyBlockFencesAreOnTheirOwnLines(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "usage:\n  cmd --flag")

	want := "**エラー詳細:**\n```\nusage:\n  cmd --flag\n```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestCodeSpan はコードスパンのヘルパーを検証します。
func TestCodeSpan(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "値を等幅で包む", in: "run_task", want: "`run_task`"},
		{name: "バックティックを ' に置き換える", in: "ls `pwd`", want: "`ls 'pwd'`"},
		{name: "空文字は空文字のまま", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.CodeSpan(tt.in); got != tt.want {
				t.Errorf("CodeSpan() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCodeSpanComposesWithField は、CodeSpan が Field と組み合わせて
// 「ラベル + 単一の値」に収まらない行を作れることを検証します。
// これが無いと呼び出し側がバックティックを直書きし、本文の記法を知る場所が
// notify パッケージの外へ漏れます。
func TestCodeSpanComposesWithField(t *testing.T) {
	b := notify.NewBody()
	b.Field("Seed", notify.CodeSpan("42")+" 🎲").
		Field("ブランチ", notify.CodeSpan("main")+" ← "+notify.CodeSpan("develop"))

	want := "**Seed:** `42` 🎲\n**ブランチ:** `main` ← `develop`"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestCodeSpanOfEmptyValueSkipsLine は、空の値を CodeSpan に通しても
// Field の「空なら行ごと省く」性質が保たれることを検証します。
func TestCodeSpanOfEmptyValueSkipsLine(t *testing.T) {
	b := notify.NewBody()
	b.Field("Seed", notify.CodeSpan(""))

	if !b.Empty() {
		t.Errorf("Empty() = false, 本文 = %q", b.String())
	}
}

// TestBodyLinkOrField は、リンク先の有無による出し分けを検証します。
func TestBodyLinkOrField(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "URL があればリンクにする",
			build: func(b *notify.Body) { b.LinkOrField("Output", "https://example.com/o", "gs://bucket/o") },
			want:  "**Output:** [gs://bucket/o](https://example.com/o)",
		},
		{
			name:  "URL が無ければ素の値にする",
			build: func(b *notify.Body) { b.LinkOrField("Output", "", "gs://bucket/o") },
			want:  "**Output:** gs://bucket/o",
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

// TestBodyLinkOrFieldSkipsEmpty は、URL も値も無い場合に行が出ないことを検証します。
func TestBodyLinkOrFieldSkipsEmpty(t *testing.T) {
	b := notify.NewBody()
	b.LinkOrField("Output", "", "")

	if !b.Empty() {
		t.Errorf("Empty() = false, 本文 = %q", b.String())
	}
}
