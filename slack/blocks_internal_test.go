package slack

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestFormatMarkdownConversions は Markdown → mrkdwn の基本変換を検証します。
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

// TestFormatMarkdownEscapesSpecialCharacters は、Slack が特殊解釈する
// & < > が実体参照に変換されることを検証します。
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

// TestFormatMarkdownSignedURLIsNotEscaped は、署名付き URL のクエリ区切り &
// がエスケープされずにそのまま残ることを検証します。
//
// Slack が & のエスケープを求めるのはプレーンテキストだけで、
// <URL|表示テキスト> の内側は対象外です。ここで &amp; に変換してしまうと
// 署名が変わって URL が 403 になるため、エスケープ範囲を誤ると壊れる側の
// 代表例として、実在の署名付き URL に近い形で固定します。
func TestFormatMarkdownSignedURLIsNotEscaped(t *testing.T) {
	const signedURL = "https://storage.example.com/bucket/artifact.bin" +
		"?Algorithm=RSA-SHA256&Expires=604800&Signature=abc123"

	got := formatMarkdown("**Artifact:** [gs://bucket/artifact.bin](" + signedURL + ")")
	want := "*Artifact:* <" + signedURL + "|gs://bucket/artifact.bin>"

	if got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownExistingLinkIsNotEscaped は、呼び出し側が直接書いた
// mrkdwn リンクの中身もエスケープされないことを検証します。
func TestFormatMarkdownExistingLinkIsNotEscaped(t *testing.T) {
	in := "<https://example.com/?a=1&b=2|結果>"

	if got := formatMarkdown(in); got != in {
		t.Errorf("formatMarkdown() = %q, want %q", got, in)
	}
}

// TestFormatMarkdownPreservesCodeBlockContent は、コードブロックの中身が
// 記法変換の対象外であることを検証します。
//
// コードブロックはエラー出力やコマンドのログを原文のまま見せるためのものなので、
// - が • に、**text** が *text* に書き換わると、貼った本人が見たい原文が壊れます。
// TestFormatMarkdownConversions とは正反対の性質を守っており、
// 片方だけ変えるともう片方が落ちます。
func TestFormatMarkdownPreservesCodeBlockContent(t *testing.T) {
	in := "**エラー詳細:**\n```\nusage:\n- foo **bar**\n## heading\n```"
	want := "*エラー詳細:*\n```\nusage:\n- foo **bar**\n## heading\n```"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownEscapesInsideCodeBlock は、コードブロックの中でも
// Slack が特殊解釈する 3 文字がエスケープされることを検証します。
// 記法変換の対象外であることと、エスケープが不要であることは別です。
func TestFormatMarkdownEscapesInsideCodeBlock(t *testing.T) {
	in := "```\nexpected <nil>, got a&b\n```"
	want := "```\nexpected &lt;nil&gt;, got a&amp;b\n```"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatMarkdownConvertsAroundCodeBlock は、コードブロックの前後が
// 通常どおり変換されることを検証します。保護範囲がブロック外へ漏れると、
// 本文の太字やリンクが素の Markdown のまま Slack に出ます。
func TestFormatMarkdownConvertsAroundCodeBlock(t *testing.T) {
	in := "**前:** [x](https://example.com)\n```\n- そのまま\n```\n- 後"
	want := "*前:* <https://example.com|x>\n```\n- そのまま\n```\n• 後"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatMarkdownUnterminatedFenceIsNotProtected は、閉じフェンスが無い場合に
// 保護を諦めて通常変換することを検証します。壊れたフェンス 1 つで以降の本文
// すべてが変換対象外になる方が、被害が大きいためです。
func TestFormatMarkdownUnterminatedFenceIsNotProtected(t *testing.T) {
	in := "```\n- item"
	want := "```\n• item"

	if got := formatMarkdown(in); got != want {
		t.Errorf("formatMarkdown() = %q, want %q", got, want)
	}
}

// TestBuildSectionTexts は、本文全体が mrkdwn に変換されることを検証します。
func TestBuildSectionTexts(t *testing.T) {
	got := buildSectionTexts(t.Context(), "## 見出し\n**重要**\n- item")
	want := []string{"*見出し*\n*重要*\n• item"}

	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("buildSectionTexts() = %q, want %q", got, want)
	}
}

// TestBuildSectionTextsEmpty は、空白のみの本文がセクションを生まないことを検証します。
func TestBuildSectionTextsEmpty(t *testing.T) {
	if got := buildSectionTexts(t.Context(), "   \n  "); len(got) != 0 {
		t.Errorf("buildSectionTexts() = %q, want none", got)
	}
}

// TestTruncateSectionText は上限を超える本文が切り詰められることを検証します。
func TestTruncateSectionText(t *testing.T) {
	long := make([]rune, maxSectionLength+100)
	for i := range long {
		long[i] = 'あ'
	}

	got := truncateSectionText(t.Context(), string(long))
	if gotLen := len([]rune(got)); gotLen > maxSectionLength {
		t.Errorf("切り詰め後の文字数 = %d, want <= %d", gotLen, maxSectionLength)
	}
}

// TestTruncateSectionTextClosesCodeFence は、コードブロックの内側で切り詰めが
// 起きた場合に閉じフェンスが補われることを検証します。
// 長い出力を貼る Body.Block が最も切り詰めに当たりやすい経路です。
func TestTruncateSectionTextClosesCodeFence(t *testing.T) {
	long := codeFence + "\n" + strings.Repeat("x", maxSectionLength+100)

	got := truncateSectionText(t.Context(), long)
	if count := strings.Count(got, codeFence); count%2 != 0 {
		t.Errorf("フェンスの数 = %d, want 偶数 (本文 = %q)", count, got[len(got)-40:])
	}
	if !strings.HasSuffix(got, codeFence) {
		t.Errorf("末尾が閉じフェンスではありません: %q", got[len(got)-40:])
	}
}

// TestTruncateHeaderText は上限を超える見出しが切り詰められることを検証します。
//
// 上限を超えたまま送ると Slack が invalid_blocks を返し、通知が丸ごと失われます。
func TestTruncateHeaderText(t *testing.T) {
	long := strings.Repeat("あ", maxHeaderLength+50)

	got := truncateHeaderText(t.Context(), long)
	if gotLen := len([]rune(got)); gotLen > maxHeaderLength {
		t.Errorf("切り詰め後の文字数 = %d, want <= %d", gotLen, maxHeaderLength)
	}
	if !strings.HasSuffix(got, headerTruncationSuffix) {
		t.Errorf("切り詰めのサフィックスが付いていません: %q", got)
	}

	short := "✅ 完了しました"
	if got := truncateHeaderText(t.Context(), short); got != short {
		t.Errorf("truncateHeaderText() = %q, want %q", got, short)
	}
}

// TestSplitSectionTextKeepsLinksIntact は、上限を超える本文が行の境界で複数ブロックに
// 分かれ、リンクの markup が途中で切れないことを検証します。署名付き URL は 1 本で
// 860 文字あり、3 本並べると 1 ブロックの上限を超えて最後のリンクが崩れていました。
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

// TestSplitSectionTextClosesAndReopensFence は、コードブロックの途中で区切るときに
// 前のブロックを閉じ、次のブロックを開き直すことを検証します。
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

// TestSplitLongLineBreaksOutsideLinks は、1 行が上限を超えるときに空白で分けつつ
// <...> の内側では分けないことを検証します。
func TestSplitLongLineBreaksOutsideLinks(t *testing.T) {
	line := "a <https://x/" + strings.Repeat("p", 30) + " q|t> b " + strings.Repeat("c", 20)
	pieces := splitLongLine(line, 40)
	if strings.Join(pieces, " ") != line {
		t.Errorf("pieces = %q, joined differs from input", pieces)
	}
	for _, p := range pieces {
		if strings.Count(p, "<") != strings.Count(p, ">") {
			t.Errorf("piece cuts a link: %q", p)
		}
	}
}

// TestBuildSectionTextsCapsBlocks は、分割しても収まらない本文は最後のブロックを
// 切り詰めることを検証します。Slack の 50 ブロック上限を超えると通知が丸ごと失われます。
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
