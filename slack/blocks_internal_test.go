package slack

import "testing"

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

// TestBuildSectionText は、本文全体が mrkdwn に変換されることを検証します。
func TestBuildSectionText(t *testing.T) {
	got := buildSectionText("## 見出し\n**重要**\n- item")
	want := "*見出し*\n*重要*\n• item"

	if got != want {
		t.Errorf("buildSectionText() = %q, want %q", got, want)
	}
}

// TestBuildSectionTextEmpty は、空白のみの本文がセクションを生まないことを検証します。
func TestBuildSectionTextEmpty(t *testing.T) {
	if got := buildSectionText("   \n  "); got != "" {
		t.Errorf("buildSectionText() = %q, want empty", got)
	}
}

// TestTruncateSectionText は上限を超える本文が切り詰められることを検証します。
func TestTruncateSectionText(t *testing.T) {
	long := make([]rune, maxSectionLength+100)
	for i := range long {
		long[i] = 'あ'
	}

	got := truncateSectionText(string(long))
	if gotLen := len([]rune(got)); gotLen > maxSectionLength {
		t.Errorf("切り詰め後の文字数 = %d, want <= %d", gotLen, maxSectionLength)
	}
}
