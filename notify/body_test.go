package notify_test

import (
	"errors"
	"testing"

	"github.com/shouni/go-notify/notify"
)

// TestBodyWriters verifies the output format of each writer.
func TestBodyWriters(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "Field bolds the label",
			build: func(b *notify.Body) { b.Field("Title", "サンプルタイトル") },
			want:  "**Title:** サンプルタイトル",
		},
		{
			name:  "Code renders the value monospace",
			build: func(b *notify.Body) { b.Code("Command", "run_task") },
			want:  "**Command:** `run_task`",
		},
		{
			name:  "Link renders a link with display text",
			build: func(b *notify.Body) { b.Link("History", "https://example.com/h/1", "job-1") },
			want:  "**History:** [job-1](https://example.com/h/1)",
		},
		{
			name:  "Link shows the URL when display text is omitted",
			build: func(b *notify.Body) { b.Link("History", "https://example.com/h/1", "") },
			want:  "**History:** [https://example.com/h/1](https://example.com/h/1)",
		},
		{
			name:  "Text writes a plain line",
			build: func(b *notify.Body) { b.Text("そのままの行") },
			want:  "そのままの行",
		},
		{
			name:  "URIField links a gs:// object to its Console details page",
			build: func(b *notify.Body) { b.URIField("Output", "gs://bucket/job/audio.wav") },
			want:  "**Output:** [gs://bucket/job/audio.wav](https://console.cloud.google.com/storage/browser/_details/bucket/job/audio.wav)",
		},
		{
			name:  "URIField links a gs:// path with a trailing slash to the bucket browser",
			build: func(b *notify.Body) { b.URIField("出力先", "gs://bucket/job/") },
			want:  "**出力先:** [gs://bucket/job/](https://console.cloud.google.com/storage/browser/bucket/job/)",
		},
		{
			name:  "URIField lists non-gs:// values as plain text",
			build: func(b *notify.Body) { b.URIField("Source", "https://example.com/article") },
			want:  "**Source:** https://example.com/article",
		},
		{
			name:  "URIField trims surrounding whitespace before checking",
			build: func(b *notify.Body) { b.URIField("Output", "  gs://bucket/obj  ") },
			want:  "**Output:** [gs://bucket/obj](https://console.cloud.google.com/storage/browser/_details/bucket/obj)",
		},
		{
			name:  "Heading renders a subheading",
			build: func(b *notify.Body) { b.Heading("生成結果") },
			want:  "## 生成結果",
		},
		{
			name:  "Bullet renders a list item",
			build: func(b *notify.Body) { b.Bullet("scene_01.png") },
			want:  "- scene_01.png",
		},
		{
			name: "Bullets can be consecutive",
			build: func(b *notify.Body) {
				b.Bullet("scene_01.png").Bullet("scene_02.png")
			},
			want: "- scene_01.png\n- scene_02.png",
		},
		{
			name: "Heading leaves a blank line after existing body",
			build: func(b *notify.Body) {
				b.Field("Title", "サンプル").Heading("生成結果").Bullet("scene_01.png")
			},
			want: "**Title:** サンプル\n\n## 生成結果\n- scene_01.png",
		},
		{
			name: "multiple lines are joined with newlines",
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

// TestBodySkipsEmptyValues verifies that fields with an empty value are not
// written. It is the central property that removes per-field presence
// checks from callers.
func TestBodySkipsEmptyValues(t *testing.T) {
	b := notify.NewBody()
	b.Field("Title", "")
	b.Code("Command", "")
	b.Link("History", "", "ラベルだけあってもURLが無い")
	b.URIField("Output", "   ")
	b.Text("")
	b.Heading("")
	b.Bullet("")

	if !b.Empty() {
		t.Fatalf("only empty values were written, yet Empty() = false, body = %q", b.String())
	}
}

// TestBodyStringFallsBackToNotAvailable verifies that a body with nothing
// written becomes NotAvailable.
func TestBodyStringFallsBackToNotAvailable(t *testing.T) {
	if got := notify.NewBody().String(); got != notify.NotAvailable {
		t.Errorf("String() = %q, want %q", got, notify.NotAvailable)
	}
}

// TestBodyZeroValueIsUsable verifies that the zero-value Body is ready to use.
func TestBodyZeroValueIsUsable(t *testing.T) {
	var b notify.Body
	b.Field("Title", "ゼロ値")

	if got, want := b.String(), "**Title:** ゼロ値"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyError verifies the error section's format and fallback.
func TestBodyError(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "nil err is N/A",
			build: func(b *notify.Body) { b.Error("エラー内容", nil) },
			want:  "**エラー内容:**\n" + notify.NotAvailable,
		},
		{
			name:  "shows the error message",
			build: func(b *notify.Body) { b.Error("エラー内容", errors.New("失敗しました")) },
			want:  "**エラー内容:**\n失敗しました",
		},
		{
			name: "separates from existing body with a blank line",
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

// TestBodyBlockSanitizesBacktick verifies that a backtick does not end the
// code block early.
func TestBodyBlockSanitizesBacktick(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "exec `run_task` failed")

	want := "**エラー詳細:**\n```\nexec 'run_task' failed\n```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyCodeSanitizesBacktick verifies that a backtick does not break the
// monospaced display.
func TestBodyCodeSanitizesBacktick(t *testing.T) {
	b := notify.NewBody()
	b.Code("Command", "ls `pwd`")

	want := "**Command:** `ls 'pwd'`"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyBlockFallsBackToNotAvailable verifies how a block with empty
// content is rendered.
func TestBodyBlockFallsBackToNotAvailable(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "")

	want := "**エラー詳細:**\n```\n" + notify.NotAvailable + "\n```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestBodyBlockFencesAreOnTheirOwnLines verifies that the fences sit on
// lines of their own.
//
// Packed onto one line as ```content```, the first content line is eaten as
// the info string and the closing fence is not at the start of a line, so
// the block never closes.
func TestBodyBlockFencesAreOnTheirOwnLines(t *testing.T) {
	b := notify.NewBody()
	b.Block("エラー詳細", "usage:\n  cmd --flag")

	want := "**エラー詳細:**\n```\nusage:\n  cmd --flag\n```"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestCodeSpan verifies the code span helper.
func TestCodeSpan(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "wraps the value in backticks", in: "run_task", want: "`run_task`"},
		{name: "replaces backticks with '", in: "ls `pwd`", want: "`ls 'pwd'`"},
		{name: "empty stays empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.CodeSpan(tt.in); got != tt.want {
				t.Errorf("CodeSpan() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCodeSpanComposesWithField verifies that CodeSpan combined with Field
// builds lines that do not fit "label + one value". Without it callers
// write backticks by hand, and knowledge of the body's markup leaks out of
// the notify package.
func TestCodeSpanComposesWithField(t *testing.T) {
	b := notify.NewBody()
	b.Field("Seed", notify.CodeSpan("42")+" 🎲").
		Field("ブランチ", notify.CodeSpan("main")+" ← "+notify.CodeSpan("develop"))

	want := "**Seed:** `42` 🎲\n**ブランチ:** `main` ← `develop`"
	if got := b.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestCodeSpanOfEmptyValueSkipsLine verifies that passing an empty value
// through CodeSpan keeps Field's "omit the line when empty" property.
func TestCodeSpanOfEmptyValueSkipsLine(t *testing.T) {
	b := notify.NewBody()
	b.Field("Seed", notify.CodeSpan(""))

	if !b.Empty() {
		t.Errorf("Empty() = false, body = %q", b.String())
	}
}

// TestBodyLinkOrField verifies the choice between a link and a plain value
// depending on whether a URL is given.
func TestBodyLinkOrField(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *notify.Body)
		want  string
	}{
		{
			name:  "links when a URL is given",
			build: func(b *notify.Body) { b.LinkOrField("Output", "https://example.com/o", "gs://bucket/o") },
			want:  "**Output:** [gs://bucket/o](https://example.com/o)",
		},
		{
			name:  "plain value without a URL",
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

// TestBodyLinkOrFieldSkipsEmpty verifies that no line is written when there
// is neither a URL nor a value.
func TestBodyLinkOrFieldSkipsEmpty(t *testing.T) {
	b := notify.NewBody()
	b.LinkOrField("Output", "", "")

	if !b.Empty() {
		t.Errorf("Empty() = false, body = %q", b.String())
	}
}
