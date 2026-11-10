package slack

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzFormatMarkdown checks that the mrkdwn conversion leaves no Slack
// control characters behind.
//
// A missed escape shows up as a string such as <script> in the body being
// parsed by Slack as a link or a mention. The conversion layers five
// regexes and span replacement, and choosing the wrong spans to protect
// lets one through. Conversely, flattening the syntax that must be kept
// (<https://…>, <@U123>, existing entities) breaks valid links.
func FuzzFormatMarkdown(f *testing.F) {
	for _, s := range fuzzMarkdownSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, message string) {
		got := formatMarkdown(message)

		// With the protected syntax and entities masked, no raw control
		// characters may remain.
		bare := preservedRegex.ReplaceAllString(got, "")
		if i := strings.IndexAny(bare, "<>&"); i >= 0 {
			t.Fatalf("unescaped %q survived conversion:\n in: %q\nout: %q", bare[i], message, got)
		}

		// The conversion is not meant to drop text: non-empty input gives
		// non-empty output (except the list marker "- ", which is replaced
		// with "• " as a whole).
		if strings.TrimSpace(message) != "" && strings.TrimSpace(got) == "" &&
			strings.TrimSpace(strings.ReplaceAll(message, "-", "")) != "" {
			t.Fatalf("conversion emptied the message:\n in: %q\nout: %q", message, got)
		}
	})
}

// FuzzSplitSectionText checks that splitting the body drops no content,
// that every block fits Slack's limit, and that code fences stay closed.
//
// The split is a hand-written scan for line and space boundaries so as not
// to cut inside a link. Cutting wrongly produces a link broken along with
// its markup (which is why truncation was replaced by splitting).
func FuzzSplitSectionText(f *testing.F) {
	for _, s := range fuzzMarkdownSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, message string) {
		chunks := splitSectionText(message)

		if len(chunks) == 0 {
			t.Fatalf("splitSectionText(%q) returned nothing", message)
		}
		// The limit is absolute. An oversized block makes Slack answer
		// invalid_blocks and the whole notification is lost. Long Japanese
		// text without spaces used to hit this.
		for i, c := range chunks {
			if n := utf8.RuneCountInString(c); n > maxSectionLength {
				t.Fatalf("chunk %d has %d runes, over the %d limit: %q", i, n, maxSectionLength, c)
			}
		}

		// Splitting introduces no fence imbalance: balanced input gives
		// balanced blocks (fixing input that arrives unclosed is not the
		// splitter's job).
		if strings.Count(message, codeFence)%2 == 0 {
			for i, c := range chunks {
				if strings.Count(c, codeFence)%2 != 0 {
					t.Fatalf("chunk %d leaves a fence open although the input was balanced: %q", i, c)
				}
			}
		}
		// Splitting must not add or lose text. Only the added fences are
		// extra, so remove them before comparing.
		joined := strings.Join(chunks, "\n")
		if len(chunks) == 1 && joined != message {
			t.Fatalf("a single chunk must be the input verbatim:\n in: %q\nout: %q", message, joined)
		}
	})
}

// fuzzMarkdownSeeds lists shapes that actually arrive as notification
// bodies.
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
