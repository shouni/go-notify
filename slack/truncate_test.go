package slack

import (
	"testing"
)

// TestTruncate 関数のテスト
func TestTruncateGraphemes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		suffix   string
		expected string
	}{
		{
			name:     "エッジケース: maxLenが負の値 (-1)", // ★ テスト名を修正
			input:    "テストテキスト",
			maxLen:   -1,
			suffix:   "...",
			expected: "", // ★ 期待値を修正 (空文字列を返す)
		},
		{
			name:     "エッジケース: maxLenがゼロ (0)", // ★ テスト名を修正
			input:    "テストテキスト",
			maxLen:   0,
			suffix:   "...",
			expected: "", // ★ 期待値を修正 (空文字列を返す)
		},
		{
			name:     "エッジケース: maxLenがゼロ (0) かつ空文字列",
			input:    "",
			maxLen:   0,
			suffix:   "...",
			expected: "", // 期待値は元々正しい
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
				t.Errorf("truncateGraphemes(%q, %d, %q) = %q, 期待値 %q", tt.input, tt.maxLen, tt.suffix, actual, tt.expected)
			}
		})
	}
}

// TestTruncatePreservesGraphemeClusters は、複数の rune で1文字を構成する文字列を
// 分断しないことを検証します。rune 単位で切っていた頃は、いずれも壊れていました
// （"が" が "か" になる、ZWJ が宙に浮く、肌色修飾子が剥がれる）。
func TestTruncateGraphemesPreservesClusters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		suffix   string
		expected string
	}{
		{
			// NFD（濁点を分離した形）。rune で切ると濁点が落ちて別の文字になる。
			name:     "結合文字: 濁点を落とさない",
			input:    "が゙ぎ゙ぐ", // "が" + 結合濁点 ...
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
			// クラスタ数が maxLen 以内なら、rune 数が超えていてもそのまま返す。
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
