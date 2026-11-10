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
			name:     "エッジケース: maxLenが負の値 (-1)", // ★ test name corrected
			input:    "テストテキスト",
			maxLen:   -1,
			suffix:   "...",
			expected: "", // ★ expected value corrected (returns an empty string)
		},
		{
			name:     "エッジケース: maxLenがゼロ (0)", // ★ test name corrected
			input:    "テストテキスト",
			maxLen:   0,
			suffix:   "...",
			expected: "", // ★ expected value corrected (returns an empty string)
		},
		{
			name:     "エッジケース: maxLenがゼロ (0) かつ空文字列",
			input:    "",
			maxLen:   0,
			suffix:   "...",
			expected: "", // the expected value was already correct
		},
		{
			name:     "最大長より短い文字列",
			input:    "Hello",
			maxLen:   10,
			suffix:   "...",
			expected: "Hello",
		},
		{
			name:     "最大長と等しい文字列",
			input:    "HelloWorld",
			maxLen:   10,
			suffix:   "...",
			expected: "HelloWorld",
		},
		{
			name:     "最大長を超える文字列 (サフィックスあり)",
			input:    "This is a long text.",
			maxLen:   10,
			suffix:   "...",
			expected: "This is a...",
		},
		{
			name:     "最大長を超える文字列 (サフィックスなし)",
			input:    "This is a long text.",
			maxLen:   10,
			suffix:   "",
			expected: "This is a",
		},
		{
			name:     "切り詰めた末尾がスペースの場合",
			input:    "ABCDEFGHI JKLM",
			maxLen:   11,
			suffix:   "...",
			expected: "ABCDEFGHI J...",
		},
		{
			name:     "空文字列",
			input:    "",
			maxLen:   5,
			suffix:   "...",
			expected: "",
		},
		{
			name:     "マルチバイト文字を含む (rune長で切り詰め)",
			input:    "あいうえお",
			maxLen:   3,
			suffix:   "...",
			expected: "あいう...",
		},
		{
			name:     "マルチバイト文字を最大長より多く指定",
			input:    "あいうえお",
			maxLen:   7,
			suffix:   "...",
			expected: "あいうえお",
		},
		{
			name:     "末尾に空白がある日本語",
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
			name:     "結合文字: 濁点を落とさない",
			input:    "が゙ぎ゙ぐ", // "が" + combining dakuten ...
			maxLen:   1,
			suffix:   "",
			expected: "が゙",
		},
		{
			name:     "ZWJ絵文字: 家族を分断しない",
			input:    "👨‍👩‍👧‍👦AB",
			maxLen:   1,
			suffix:   "",
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "ZWJ絵文字: 2文字目まで",
			input:    "👨‍👩‍👧‍👦AB",
			maxLen:   2,
			suffix:   "...",
			expected: "👨‍👩‍👧‍👦A...",
		},
		{
			name:     "肌色修飾子を剥がさない",
			input:    "👋🏽👋🏽👋🏽",
			maxLen:   1,
			suffix:   "",
			expected: "👋🏽",
		},
		{
			// Within maxLen clusters, return unchanged even if the rune count
			// exceeds it.
			name:     "クラスタ数が上限以内なら切らない",
			input:    "👨‍👩‍👧‍👦",
			maxLen:   2,
			suffix:   "...",
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "サロゲートペア単体",
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
