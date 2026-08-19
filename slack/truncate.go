package slack

import (
	"strings"

	"github.com/rivo/uniseg"
)

// truncateGraphemes は、指定された文字列を書記素クラスタ（人間が「1文字」と認識する単位）の数で
// 最大長まで切り詰め、必要に応じてサフィックスを追加します。maxLen が 0 以下の場合は
// サフィックスを付けずに空文字列を返します。
//
// rune 単位ではなく書記素クラスタ単位で数えるのは、複数の rune が合わさって 1 文字を
// 構成するケースを壊さないためです。rune で切ると次のような破壊が起きます。
//
//	"がぎぐけこ"（濁点を分離した NFD 形）を 1 で切ると "か" になり、濁点が消えて別の語になる
//	"👨‍👩‍👧‍👦"（ZWJ 絵文字）が途中で分断され、宙に浮いた結合子が残る
//	"👋🏽"（肌色修飾子付き）から修飾子が剥がれ、別の見た目になる
//
// 注意: 切り詰められた文字列の末尾に空白文字が残った場合、サフィックスを付加する前に
// strings.TrimSpaceを使用してその末尾の空白は無条件に削除されます。
// これは、ログ出力や表示用途で不要な末尾スペースを排除し、サフィックスを綺麗に付加するための仕様です
func truncateGraphemes(s string, maxLen int, suffix string) string {
	if maxLen <= 0 {
		return ""
	}

	// 先頭から書記素クラスタを1つずつ取り出し、maxLen 個ぶんのバイト長を測る。
	// クラスタ数を数えてから改めて切り直すより、走査が1回で済む。
	count := 0
	offset := 0
	rest := s
	state := -1
	for rest != "" {
		if count == maxLen {
			// maxLen 個を取り終えてもまだ残りがある = 切り詰めが必要。
			break
		}
		var cluster string
		cluster, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		count++
		offset += len(cluster)
	}

	// 最後まで走査しきった（＝全体が maxLen 以内）なら元の文字列をそのまま返す。
	if rest == "" {
		return s
	}

	return strings.TrimSpace(s[:offset]) + suffix
}
