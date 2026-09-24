package slack

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzFormatMarkdown は、mrkdwn への変換が Slack の制御文字を残さないことを確かめます。
//
// エスケープ漏れは、本文に書かれた <script> のような文字列が Slack 側でリンクや
// メンションとして解釈される形で出ます。変換は正規表現 5 本と区間の置換を重ねており、
// 「どの区間を守るか」を間違えると漏れます。逆に守るべき構文（<https://…> や <@U123>、
// 既存の実体参照）まで潰すと、正しいリンクが壊れます。
func FuzzFormatMarkdown(f *testing.F) {
	for _, s := range fuzzMarkdownSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, message string) {
		got := formatMarkdown(message)

		// 守る構文と実体参照を伏せたうえで、生の制御文字が残っていないこと。
		bare := preservedRegex.ReplaceAllString(got, "")
		if i := strings.IndexAny(bare, "<>&"); i >= 0 {
			t.Fatalf("unescaped %q survived conversion:\n in: %q\nout: %q", bare[i], message, got)
		}

		// 変換は文字を落とすためのものではない。入力が空でなければ出力も空でない
		// （リスト記法の "- " だけは記号ごと "• " へ置き換わるので除く）。
		if strings.TrimSpace(message) != "" && strings.TrimSpace(got) == "" &&
			strings.TrimSpace(strings.ReplaceAll(message, "-", "")) != "" {
			t.Fatalf("conversion emptied the message:\n in: %q\nout: %q", message, got)
		}
	})
}

// FuzzSplitSectionText は、本文の分割が内容を落とさず、各ブロックが Slack の上限に
// 収まり、コードフェンスが閉じたままであることを確かめます。
//
// 分割はリンクの途中で切らないために行と空白の境界を探す手書きの走査です。切り方を
// 誤ると markup ごと壊れたリンクが出ます（それを避けるために切り詰めをやめて分割にした）。
func FuzzSplitSectionText(f *testing.F) {
	for _, s := range fuzzMarkdownSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, message string) {
		chunks := splitSectionText(message)

		if len(chunks) == 0 {
			t.Fatalf("splitSectionText(%q) returned nothing", message)
		}
		// 上限は絶対に守る。超えたブロックを送ると Slack が invalid_blocks を返し、
		// 通知が丸ごと失われる。空白の無い日本語の長文がここに当たっていた。
		for i, c := range chunks {
			if n := utf8.RuneCountInString(c); n > maxSectionLength {
				t.Fatalf("chunk %d has %d runes, over the %d limit: %q", i, n, maxSectionLength, c)
			}
		}

		// 分割はフェンスの不均衡を持ち込まない。入力が均衡していれば各ブロックも均衡する
		// （入力そのものが開きっぱなしの場合まで直すのは分割の仕事ではない）。
		if strings.Count(message, codeFence)%2 == 0 {
			for i, c := range chunks {
				if strings.Count(c, codeFence)%2 != 0 {
					t.Fatalf("chunk %d leaves a fence open although the input was balanced: %q", i, c)
				}
			}
		}
		// 分割で本文が増減しないこと。フェンスの補いだけは足されるので、
		// それを取り除いてから比べる。
		joined := strings.Join(chunks, "\n")
		if len(chunks) == 1 && joined != message {
			t.Fatalf("a single chunk must be the input verbatim:\n in: %q\nout: %q", message, joined)
		}
	})
}

// fuzzMarkdownSeeds は、通知本文として実際に来る形を並べたものです。
func fuzzMarkdownSeeds() []string {
	return []string{
		"本文 **強調**\n- item\n## 見出し",
		"[text](https://example.com/a_(b)_c)",
		"<https://example.com|already mrkdwn>",
		"<@U12345> と <#C12345>",
		"&amp; &lt; &gt;",
		"生の < と > と &",
		"```\ncode <with> &entities;\n```",
		"```json\n{\"a\":1}\n```\nafter",
		"```\nunclosed fence",
		"*Result:* <https://storage.googleapis.com/" + strings.Repeat("x", 900) + "|link>",
		strings.Repeat("あ", maxSectionLength+50),
		strings.Repeat("word ", 900),
		"",
		"-",
		"\n\n\n",
	}
}
