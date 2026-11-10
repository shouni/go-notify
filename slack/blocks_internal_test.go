package slack

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestFormatMarkdownConversions verifies the basic Markdown → mrkdwn
// conversions.
func TestFormatMarkdownConversions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "太字は1つのアスタリスクになる",
			in:   "**重要**",
			want: "*重要*",
		},
		{
			name: "見出しは太字になる",
			in:   "## 見出し",
			want: "*見出し*",
		},
		{
			name: "リストは中黒になる",
			in:   "- item",
			want: "• item",
		},
		{
			name: "リンクは <URL|表示テキスト> になる",
			in:   "[job-1](https://example.com/h/1)",
			want: "<https://example.com/h/1|job-1>",
		},
		{
			name: "URL に含まれる対応の取れた括弧はリンクの一部として扱う",
			in:   "[詳細](https://example.com/a_(b)_c)",
			want: "<https://example.com/a_(b)_c|詳細>",
		},
		{
			name: "対応の取れない括弧を含む URL は Markdown のまま残す",
			in:   "[詳細](https://example.com/a_(b_c)",
			want: "[詳細](https://example.com/a_(b_c)",
		},
		{
			name: "既に mrkdwn のリンクはそのまま通る",
			in:   "<https://example.com/h/1|job-1>",
			want: "<https://example.com/h/1|job-1>",
		},
		{
			name: "メンションはエスケープされない",
			in:   "<@U012AB3CD> さん、<!here>",
			want: "<@U012AB3CD> さん、<!here>",
		},
		{
			name: "インラインコードはそのまま通る",
			in:   "`run_task`",
			want: "`run_task`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMarkdown(tt.in); got != tt.want {
				t.Errorf("formatMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatMarkdownEscapesSpecialCharacters verifies that & < >, which
// Slack treats specially, are converted to entities.
func TestFormatMarkdownEscapesSpecialCharacters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "アンパサンドをエスケープする",
			in:   "R&D チーム",
			want: "R&amp;D チーム",
		},
		{
			name: "不等号をエスケープする",
			in:   "expected <nil>, got 5 > 3",
			want: "expected &lt;nil&gt;, got 5 &gt; 3",
		},
		{
			name: "実体参照は二重エスケープしない",
			in:   "R&amp;D",
			want: "R&amp;D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMarkdown(tt.in); got != tt.want {
				t.Errorf("formatMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatMarkdownSignedURLIsNotEscaped verifies that the & query
// separators of a signed URL are left unescaped.
//
// Slack requires & to be escaped in plain text only, not inside
// <URL|text>. Converting it to &amp; here changes the signature and the
// URL answers 403, so as the typical case that breaks when the escaping
// scope is wrong, it is pinned in a shape close to a real signed URL.
func TestFormatMarkdownSignedURLIsNotEscaped(t *testing.T) {
	const signedURL = "https://storage.example.com/bucket/artifact.bin" +
		"?Algorithm=RSA-SHA256&Expires=604800&Signature=abc123"

	got := formatMarkdown("**Artifact:** [gs://bucket/artifact.bin](" + signedURL + ")")
	want := "*Artifact:* <" + signedURL + "|gs://bucket/artifact.bin>"

	if got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownExistingLinkIsNotEscaped verifies that the inside of a
// mrkdwn link written directly by the caller is not escaped either.
func TestFormatMarkdownExistingLinkIsNotEscaped(t *testing.T) {
	in := "<https://example.com/?a=1&b=2|結果>"

	if got := formatMarkdown(in); got != in {
		t.Errorf("formatMarkdown() = %q, want %q", got, in)
	}
}

// TestFormatMarkdownPreservesCodeBlockContent verifies that code block
// contents are exempt from markup conversion.
//
// A code block shows error output or command logs verbatim, so rewriting -
// to • or **text** to *text* breaks the very text its author wants to see.
// It guards the opposite property to TestFormatMarkdownConversions, and
// changing one side alone makes the other fail.
func TestFormatMarkdownPreservesCodeBlockContent(t *testing.T) {
	in := "**エラー詳細:**\n```\nusage:\n- foo **bar**\n## heading\n```"
	want := "*エラー詳細:*\n```\nusage:\n- foo **bar**\n## heading\n```"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownEscapesInsideCodeBlock verifies that the three
// characters Slack treats specially are escaped inside code blocks too.
// Being exempt from markup conversion is not the same as needing no
// escaping.
func TestFormatMarkdownEscapesInsideCodeBlock(t *testing.T) {
	in := "```\nexpected <nil>, got a&b\n```"
	want := "```\nexpected &lt;nil&gt;, got a&amp;b\n```"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatMarkdownConvertsAroundCodeBlock verifies that the text before
// and after a code block is converted as usual. If the protected range
// leaked outside the block, the body's bold and links would reach Slack as
// raw Markdown.
func TestFormatMarkdownConvertsAroundCodeBlock(t *testing.T) {
	in := "**前:** [x](https://example.com)\n```\n- そのまま\n```\n- 後"
	want := "*前:* <https://example.com|x>\n```\n- そのまま\n```\n• 後"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownUnterminatedFenceIsNotProtected verifies that without a
// closing fence the protection is dropped and the text is converted as
// usual. One broken fence exempting the rest of the body would do more
// damage.
func TestFormatMarkdownUnterminatedFenceIsNotProtected(t *testing.T) {
	in := "```\n- item"
	want := "```\n• item"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() = %q, want %q", got, want)
	}
}

// TestBuildSectionTexts verifies that the whole body is converted to mrkdwn.
func TestBuildSectionTexts(t *testing.T) {
	got := buildSectionTexts(t.Context(), "## 見出し\n**重要**\n- item")
	want := []string{"*見出し*\n*重要*\n• item"}

	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("buildSectionTexts() = %q, want %q", got, want)
	}
}

// TestBuildSectionTextsEmpty verifies that a whitespace-only body produces
// no section.
func TestBuildSectionTextsEmpty(t *testing.T) {
	if got := buildSectionTexts(t.Context(), "   \n  "); len(got) != 0 {
		t.Errorf("buildSectionTexts() = %q, want none", got)
	}
}

// TestTruncateSectionText verifies that a body over the limit is truncated.
func TestTruncateSectionText(t *testing.T) {
	long := make([]rune, maxSectionLength+100)
	for i := range long {
		long[i] = 'あ'
	}

	got := truncateSectionText(t.Context(), string(long))
	if gotLen := len([]rune(got)); gotLen > maxSectionLength {
		t.Errorf("length after truncation = %d, want <= %d", gotLen, maxSectionLength)
	}
}

// TestTruncateSectionTextClosesCodeFence verifies that a closing fence is
// added when truncation happens inside a code block. Body.Block, which
// carries long pasted output, is the path most likely to be truncated.
func TestTruncateSectionTextClosesCodeFence(t *testing.T) {
	long := codeFence + "\n" + strings.Repeat("x", maxSectionLength+100)

	got := truncateSectionText(t.Context(), long)
	if count := strings.Count(got, codeFence); count%2 != 0 {
		t.Errorf("fence count = %d, want even (body = %q)", count, got[len(got)-40:])
	}
	if !strings.HasSuffix(got, codeFence) {
		t.Errorf("does not end with a closing fence: %q", got[len(got)-40:])
	}
}

// TestTruncateHeaderText verifies that a heading over the limit is
// truncated.
//
// Sent over the limit, Slack answers invalid_blocks and the whole
// notification is lost.
func TestTruncateHeaderText(t *testing.T) {
	long := strings.Repeat("あ", maxHeaderLength+50)

	got := truncateHeaderText(t.Context(), long)
	if gotLen := len([]rune(got)); gotLen > maxHeaderLength {
		t.Errorf("length after truncation = %d, want <= %d", gotLen, maxHeaderLength)
	}
	if !strings.HasSuffix(got, headerTruncationSuffix) {
		t.Errorf("truncation suffix missing: %q", got)
	}

	short := "✅ 完了しました"
	if got := truncateHeaderText(t.Context(), short); got != short {
		t.Errorf("truncateHeaderText() = %q, want %q", got, short)
	}
}

// TestSplitSectionTextKeepsLinksIntact verifies that a body over the limit
// is split across blocks at line boundaries without cutting a link's
// markup. A signed URL is 860 characters, and three of them crossed one
// block's limit and broke the last link.
func TestSplitSectionTextKeepsLinksIntact(t *testing.T) {
	link := func(n int) string {
		return "<https://storage.googleapis.com/b/" + strings.Repeat("x", 1000) + "?sig=" + strings.Repeat("a", 40) + "|link" + string(rune('0'+n)) + ">"
	}
	body := "*Result:* " + link(1) + "\n*Master:* " + link(2) + "\n*Recipe:* " + link(3) + "\n*Audio Check:* ok"

	chunks := splitSectionText(body)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want the body split across blocks", len(chunks))
	}
	joined := strings.Join(chunks, "\n")
	if joined != body {
		t.Errorf("splitting lost or altered content:\n got %q\nwant %q", joined, body)
	}
	for i, c := range chunks {
		if n := utf8.RuneCountInString(c); n > maxSectionLength {
			t.Errorf("chunk %d has %d runes, want <= %d", i, n, maxSectionLength)
		}
		if strings.Count(c, "<") != strings.Count(c, ">") {
			t.Errorf("chunk %d cuts a link: %q", i, c)
		}
	}
}

// TestSplitSectionTextClosesAndReopensFence verifies that a split inside a
// code block closes the previous block and reopens the next.
func TestSplitSectionTextClosesAndReopensFence(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("before\n" + codeFence + "\n")
	for range 200 {
		sb.WriteString(strings.Repeat("y", 40) + "\n")
	}
	sb.WriteString(codeFence + "\nafter")

	chunks := splitSectionText(sb.String())
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want the block split", len(chunks))
	}
	for i, c := range chunks {
		if strings.Count(c, codeFence)%2 != 0 {
			t.Errorf("chunk %d has an unbalanced fence: starts %q ends %q", i, c[:min(20, len(c))], c[max(0, len(c)-20):])
		}
		if n := utf8.RuneCountInString(c); n > maxSectionLength {
			t.Errorf("chunk %d has %d runes, want <= %d", i, n, maxSectionLength)
		}
	}
	if !strings.HasSuffix(chunks[len(chunks)-1], "after") {
		t.Errorf("text after the block was lost: %q", chunks[len(chunks)-1])
	}
}

// TestSplitLongLineBreaksOutsideLinks verifies that a line is split without
// cutting a link that fits the limit. Cuts prefer spaces and avoid the
// inside of <...>.
func TestSplitLongLineBreaksOutsideLinks(t *testing.T) {
	line := "a <https://x/" + strings.Repeat("p", 20) + "|t> b " + strings.Repeat("c", 30)
	pieces := splitLongLine(line, 40)

	if strings.Join(pieces, " ") != line {
		t.Errorf("pieces = %q, joined differs from input", pieces)
	}
	for _, p := range pieces {
		if strings.Count(p, "<") != strings.Count(p, ">") {
			t.Errorf("piece cuts a link that would have fit: %q", p)
		}
	}
}

// TestSplitLongLineCutsAnOversizedLink verifies that a single link over the
// limit is cut anyway.
//
// Keeping the link whole and returning an oversized block makes Slack
// answer invalid_blocks, and the whole notification is lost. One broken
// link is the lighter failure. A signed URL is about 900 characters, so
// only abnormal input gets here, but that is no reason to lose the
// notification when it does.
func TestSplitLongLineCutsAnOversizedLink(t *testing.T) {
	const maxLen = 40
	line := "<https://x/" + strings.Repeat("p", 100) + "|t>"

	pieces := splitLongLine(line, maxLen)

	if strings.Join(pieces, "") != line {
		t.Errorf("pieces = %q, joined differs from input", pieces)
	}
	for i, p := range pieces {
		if n := utf8.RuneCountInString(p); n > maxLen {
			t.Errorf("piece %d has %d runes, over the %d limit", i, n, maxLen)
		}
	}
}

// TestBuildSectionTextsCapsBlocks verifies that a body that does not fit
// even when split has its last block truncated. Exceeding Slack's 50-block
// limit loses the whole notification.
func TestBuildSectionTextsCapsBlocks(t *testing.T) {
	body := strings.Repeat(strings.Repeat("z", 100)+"\n", maxSectionBlocks*40)
	got := buildSectionTexts(t.Context(), body)
	if len(got) != maxSectionBlocks {
		t.Errorf("blocks = %d, want %d", len(got), maxSectionBlocks)
	}
	if !strings.Contains(got[len(got)-1], "省略されました") {
		t.Errorf("last block lacks the truncation note: %q", got[len(got)-1][len(got[len(got)-1])-60:])
	}
}
