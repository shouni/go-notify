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
			in:   "`compose_video`",
			want: "`compose_video`",
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
// 代表例として、実際の GCS 署名付き URL に近い形で固定します。
func TestFormatMarkdownSignedURLIsNotEscaped(t *testing.T) {
	const signedURL = "https://storage.googleapis.com/bucket/song.wav" +
		"?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Expires=604800&X-Goog-Signature=abc123"

	got := formatMarkdown("**Audio File:** [gs://bucket/song.wav](" + signedURL + ")")
	want := "*Audio File:* <" + signedURL + "|gs://bucket/song.wav>"

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
