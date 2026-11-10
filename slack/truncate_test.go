package slack

import (
	"testing"
)

// TestTruncateGraphemes tests truncateGraphemes.
func TestTruncateGraphemes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		suffix   string
		expected string
	}{
		{
			name:     "edge case: negative maxLen (-1)", // ★ test name corrected
			input:    "テストテキスト",
			maxLen:   -1,
			suffix:   "...",
			expected: "", // ★ expected value corrected (returns an empty string)
		},
		{
			name:     "edge case: zero maxLen (0)", // ★ test name corrected
			input:    "テストテキスト",
			maxLen:   0,
			suffix:   "...",
			expected: "", // ★ expected value corrected (returns an empty string)
		},
		{
			name:     "edge case: zero maxLen (0) with empty string",
			input:    "",
			maxLen:   0,
			suffix:   "...",
			expected: "", // the expected value was already correct
		},
		{
			name:     "shorter than max length",
			input:    "Hello",
			maxLen:   10,
			suffix:   "...",
			expected: "Hello",
		},
		{
			name:     "equal to max length",
			input:    "HelloWorld",
			maxLen:   10,
			suffix:   "...",
			expected: "HelloWorld",
		},
		{
			name:     "longer than max length (with suffix)",
			input:    "This is a long text.",
			maxLen:   10,
			suffix:   "...",
			expected: "This is a...",
		},
		{
			name:     "longer than max length (no suffix)",
			input:    "This is a long text.",
			maxLen:   10,
			suffix:   "",
			expected: "This is a",
		},
		{
			name:     "truncation ends on a space",
			input:    "ABCDEFGHI JKLM",
			maxLen:   11,
			suffix:   "...",
			expected: "ABCDEFGHI J...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   5,
			suffix:   "...",
			expected: "",
		},
		{
			name:     "multibyte text (truncated by rune length)",
			input:    "あいうえお",
			maxLen:   3,
			suffix:   "...",
			expected: "あいう...",
		},
		{
			name:     "multibyte text with max length above its length",
			input:    "あいうえお",
			maxLen:   7,
			suffix:   "...",
			expected: "あいうえお",
		},
		{
			name:     "Japanese with trailing whitespace",
			input:    "テストテキスト　　です。 ",
			maxLen:   6,
			suffix:   "...",
			expected: "テストテキス...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := truncateGraphemes(tt.input, tt.maxLen, tt.suffix)
			if actual != tt.expected {
				t.Errorf("truncateGraphemes(%q, %d, %q) = %q, want %q", tt.input, tt.maxLen, tt.suffix, actual, tt.expected)
			}
		})
	}
}

// TestTruncatePreservesGraphemeClusters verifies that characters made of
// several runes are not split. While truncation cut by rune, every one of
// these broke ("が" became "か", a ZWJ was left dangling, a skin-tone
// modifier came off).
func TestTruncateGraphemesPreservesClusters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		suffix   string
		expected string
	}{
		{
			// NFD (dakuten separated). Cutting by rune drops the dakuten and
			// yields a different character.
			name:     "combining mark: keeps the dakuten",
			input:    "が゙ぎ゙ぐ", // "が" + combining dakuten ...
			maxLen:   1,
			suffix:   "",
			expected: "が゙",
		},
		{
			name:     "ZWJ emoji: does not split the family",
			input:    "👨‍👩‍👧‍👦AB",
			maxLen:   1,
			suffix:   "",
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "ZWJ emoji: up to the second character",
			input:    "👨‍👩‍👧‍👦AB",
			maxLen:   2,
			suffix:   "...",
			expected: "👨‍👩‍👧‍👦A...",
		},
		{
			name:     "keeps the skin tone modifier",
			input:    "👋🏽👋🏽👋🏽",
			maxLen:   1,
			suffix:   "",
			expected: "👋🏽",
		},
		{
			// Within maxLen clusters, return unchanged even if the rune count
			// exceeds it.
			name:     "no truncation within the cluster limit",
			input:    "👨‍👩‍👧‍👦",
			maxLen:   2,
			suffix:   "...",
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "single surrogate pair",
			input:    "𠮷野家",
			maxLen:   2,
			suffix:   "…",
			expected: "𠮷野…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateGraphemes(tt.input, tt.maxLen, tt.suffix); got != tt.expected {
				t.Errorf("truncateGraphemes(%q, %d, %q) = %q, want %q",
					tt.input, tt.maxLen, tt.suffix, got, tt.expected)
			}
		})
	}
}
