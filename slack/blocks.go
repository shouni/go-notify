// Package slack は、notify.Notifier の Slack Incoming Webhook 実装を提供します。
//
// 本文の標準 Markdown を Slack mrkdwn へ変換し、Block Kit 形式に組み立てて投稿します。
// 入口は NewNotifier だけで、投稿処理そのものは公開していません。
package slack

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"github.com/shouni/go-utils/jst"
)

const (
	// maxHeaderLength は Slack ヘッダーブロックの最大文字数です。Slack の上限そのものです。
	maxHeaderLength = 150
	// maxSectionBlocks は本文を分割して並べるセクションブロックの上限数です。
	// Slack はメッセージ 1 件につき 50 ブロックまでで、見出し・区切り・フッターに
	// 3 つ使います。通知として読める長さの上限でもあるので、余裕を持って抑えます。
	maxSectionBlocks = 20
	// maxSectionLength は Slack セクションブロックの最大文字数です。
	// Slack の上限 3000 の手前に余裕を取っています。
	maxSectionLength = 2900
	// headerTruncationSuffix は見出し切り捨て時に追加するサフィックスです。
	headerTruncationSuffix = "…"
	// truncationSuffix は本文切り捨て時に追加するサフィックスです。
	truncationSuffix = "\n\n... (メッセージが長すぎるため省略されました)"
	// codeFence は本文中のコードブロックのフェンスです。
	// notify.Body が出力する並びと一致している必要があります（片方だけ変えると検出が外れます）。
	codeFence = "```"
)

var (
	// boldRegex は Markdown の太字記法を Slack mrkdwn の太字記法に変換します。
	boldRegex = regexp.MustCompile(`\*\*(.*?)\*\*`) // **text** -> *text*
	// headerRegex は Markdown の見出しを Slack mrkdwn の太字記法に変換します。
	headerRegex = regexp.MustCompile(`(?m)^##\s*(.*)$`) // ## Title -> *Title*
	// listItemRegex は Markdown のリスト項目を Slack mrkdwn 向けの箇条書きに変換します。
	// 空白は同一行のものだけを対象にします（\s だと改行まで飲み込み、
	// 直前の行との改行ごと箇条書きに置き換えてしまうため）。
	listItemRegex = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+`) // - item -> • item
	// linkRegex は Markdown のリンクを Slack mrkdwn のリンクに変換します。
	//
	// URL 側は括弧の対応が取れていれば 1 段まで含められます。閉じ括弧を
	// 「最初に見つかった )」で決めると、a_(b)_c のような URL が途中で切れ、
	// 表示テキストと混ざった壊れたリンクになります。
	// 対応の取れない括弧を含む URL と、表示テキストに ] を含むリンクは
	// マッチせず、Markdown のまま出ます（壊れたリンクにするより literal のほうが読めるため）。
	linkRegex = regexp.MustCompile(`\[([^\]]*)\]\(((?:[^()\s]|\([^()\s]*\))+)\)`) // [text](url) -> <url|text>
	// fencedBlockRegex は行頭のフェンスで開き、行頭のフェンスで閉じるコードブロックにマッチします。
	// 閉じフェンスが無い場合はマッチせず、通常のテキストとして変換されます
	// （壊れたフェンスに引きずられて以降の本文すべてが変換対象外になるのを避けるため）。
	fencedBlockRegex = regexp.MustCompile("(?ms)^" + codeFence + "[^\n]*\n.*?^" + codeFence + "[ \t]*$")
	// preservedRegex は、エスケープしてはいけない部分にマッチします。
	//
	// 対象は Slack が構文として解釈する <...>、すなわちスキーム付きリンク
	// （<https://…|表示テキスト>、<mailto:…>）とメンション（<@U123>、<#C123>、
	// <!here>）、および既にエスケープ済みの実体参照です。
	//
	// <...> の中身を無条件に残さないのは、エラー文の <nil> のような
	// ただの不等号までリンク構文と誤認してしまうためです。スキームか
	// メンション記号で始まるものだけを構文として扱います。
	preservedRegex = regexp.MustCompile(`<(?:[a-zA-Z][a-zA-Z0-9+.\-]*:[^<>]*|[@#!][^<>]*)>|&(?:amp|lt|gt);`)
)

// mrkdwnEscaper は Slack が特殊解釈する 3 文字を実体参照へ置き換えます。
// strings.Replacer は置換結果を再走査しないため、& の二重エスケープは起きません。
var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// buildMessageBlocks は Slack の Block Kit ブロックを構築します。
// 見出しと本文はそれぞれのブロックの上限に収まるよう切り詰めます。
func buildMessageBlocks(ctx context.Context, headerText string, message string) ([]block, error) {
	if headerText == "" {
		return nil, errors.New("通知の見出しが空です")
	}

	blocks := []block{
		headerBlock(truncateHeaderText(ctx, headerText)),
		dividerBlock(),
	}

	for _, sectionText := range buildSectionTexts(ctx, message) {
		blocks = append(blocks, sectionBlock(sectionText))
	}

	blocks = append(blocks, buildFooterBlock())

	return blocks, nil
}

// buildSectionTexts は本文を Slack セクションブロック用の mrkdwn 文字列に変換し、
// 1 ブロックの上限に収まるよう行の境界で分割します。空の本文は 0 個です。
//
// 切り詰めではなく分割するのは、切った位置がリンクの途中だと markup ごと壊れるためです
// （署名付き URL は 1 本で 860 文字あり、3 本並べただけで上限を超えて最後のリンクが
// 崩れていました）。分割しても maxSectionBlocks に収まらないときだけ、最後のブロックを
// 切り詰めます。
func buildSectionTexts(ctx context.Context, message string) []string {
	if strings.TrimSpace(message) == "" {
		return nil
	}

	chunks := splitSectionText(formatMarkdown(message))
	if len(chunks) <= maxSectionBlocks {
		return chunks
	}

	slog.WarnContext(ctx, "The notification message is too long even when split, truncating.",
		"blocks", len(chunks),
		"max_blocks", maxSectionBlocks)
	chunks = chunks[:maxSectionBlocks]
	last := &chunks[len(chunks)-1]
	// 最後のブロックは上限以内なので、省略の注記ぶんだけ空けてから注記を必ず付けます。
	// フェンスは注記より前で閉じます（注記がコードブロックの中に入らないように）。
	room := maxSectionLength - utf8.RuneCountInString(truncationSuffix) - fenceReserve
	*last = closeUnterminatedFence(truncateGraphemes(*last, room, "")) + truncationSuffix
	return chunks
}

// fenceReserve は、コードブロックの途中で区切るときに閉じフェンスのために空けておく文字数です。
const fenceReserve = len("\n" + codeFence)

// maxPieceLength は、1 行を分けるときの 1 片の上限です。
//
// フェンスの途中で区切ると、そのブロックは「開きフェンス + 改行 + 本文 + 改行 + 閉じフェンス」
// になります。予算から引くのは閉じ側（fenceReserve）だけでは足りず、再開する開きフェンスと
// その直後の改行も要ります。ここを 1 文字でも見誤ると、上限を超えたブロックが送られて
// Slack が invalid_blocks を返し、通知が丸ごと失われます。
const maxPieceLength = maxSectionLength - 2*fenceReserve

// splitSectionText は mrkdwn の本文を、各要素が maxSectionLength 以内になるよう
// 行の境界で分割します。コードブロックの途中で区切るときは、前の要素を閉じフェンスで
// 閉じ、次の要素を開きフェンスで始めます。1 行が上限を超えるときは空白で分けます。
func splitSectionText(message string) []string {
	if utf8.RuneCountInString(message) <= maxSectionLength {
		return []string{message}
	}

	var (
		chunks  []string
		cur     strings.Builder
		curLen  int
		inFence bool
	)
	flush := func() {
		text := cur.String()
		cur.Reset()
		curLen = 0
		if inFence {
			text += "\n" + codeFence
			cur.WriteString(codeFence)
			curLen = utf8.RuneCountInString(codeFence)
		}
		chunks = append(chunks, text)
	}

	for line := range strings.SplitSeq(message, "\n") {
		for _, piece := range splitLongLine(line, maxPieceLength) {
			pieceLen := utf8.RuneCountInString(piece)
			sep := 0
			if curLen > 0 {
				sep = 1
			}
			reserve := 0
			if inFence || isFenceLine(piece) {
				reserve = fenceReserve
			}
			if curLen > 0 && curLen+sep+pieceLen+reserve > maxSectionLength {
				flush()
				sep = 0
				if curLen > 0 {
					sep = 1
				}
			}
			if sep == 1 {
				cur.WriteByte('\n')
			}
			cur.WriteString(piece)
			curLen += sep + pieceLen
			if isFenceLine(piece) {
				inFence = !inFence
			}
		}
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// isFenceLine は、行がコードブロックの開き・閉じフェンスかを返します。
func isFenceLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), codeFence)
}

// splitLongLine は、1 行が maxLen を超えるときに分けます。
//
// 切る位置は、空白 → リンクの外側の任意の位置、の順に探します。空白を優先するのは
// 語の途中で切らないためですが、**日本語は単語間に空白を置かない**ので、空白が無いのは
// 例外ではなく通常です。そこで諦めると上限を超えたブロックがそのまま送られ、Slack が
// invalid_blocks を返して通知が丸ごと失われます（見出しの切り詰めと同じ失敗の形です）。
//
// リンク構文 <...> の内側は避けますが、窓が丸ごとリンクの内側なら割ってでも切ります。
// 壊れたリンクが 1 本出るのは、上限超えで通知が丸ごと消えるより軽い失敗だからです
// （署名付き URL は 900 文字程度なので、実際にここへ来るのは異常な入力だけです）。
func splitLongLine(line string, maxLen int) []string {
	if utf8.RuneCountInString(line) <= maxLen {
		return []string{line}
	}

	var pieces []string
	for utf8.RuneCountInString(line) > maxLen {
		cut := lastBreakableSpace(line, maxLen)
		if cut <= 0 {
			// 空白が無いので境界で切る。リンクの外側を優先し、窓が丸ごとリンクの
			// 内側ならリンクを割ってでも切る。
			outside, anywhere := lastBreaks(line, maxLen)
			if cut = outside; cut <= 0 {
				cut = anywhere
			}
			if cut <= 0 {
				break // 1 クラスタが上限を超えている。これ以上は分けられない。
			}
			pieces = append(pieces, line[:cut])
			line = line[cut:]
			continue
		}
		pieces = append(pieces, strings.TrimRight(line[:cut], " "))
		line = strings.TrimLeft(line[cut:], " ")
	}
	return append(pieces, line)
}

// lastBreaks は、先頭から maxLen 文字以内にある最後の書記素クラスタ境界を 2 つ返します。
// outside はリンク構文 <...> の外側にあるもの（無ければ -1）、anywhere は位置を問わない
// 最後のものです。どちらもバイト位置です。
//
// 書記素クラスタの境界を選ぶのは、結合文字や ZWJ 絵文字を分断しないためです
// （truncateGraphemes と同じ理由）。数えるのはルーンです。上限の判定が
// utf8.RuneCountInString である以上、ここだけクラスタで数えると、1 クラスタが
// 複数ルーンになる入力（結合文字列や不正な UTF-8）で上限を 1 文字超えます。
func lastBreaks(line string, maxLen int) (outside, anywhere int) {
	outside, anywhere = -1, -1
	depth, runes, offset := 0, 0, 0
	rest, state := line, -1
	for rest != "" {
		var cluster string
		cluster, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		if runes+utf8.RuneCountInString(cluster) > maxLen {
			break
		}
		switch cluster {
		case "<":
			depth++
		case ">":
			if depth > 0 {
				depth--
			}
		}
		runes += utf8.RuneCountInString(cluster)
		offset += len(cluster)
		anywhere = offset
		if depth == 0 {
			outside = offset
		}
	}
	return outside, anywhere
}

// lastBreakableSpace は、先頭から maxLen 文字以内にある最後の空白のバイト位置を返します。
// <...> の内側にある空白は候補にしません。無ければ -1 です。
func lastBreakableSpace(line string, maxLen int) int {
	cut := -1
	depth := 0
	runes := 0
	for i, r := range line {
		if runes >= maxLen {
			break
		}
		runes++
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ' ':
			if depth == 0 {
				cut = i
			}
		}
	}
	return cut
}

// formatMarkdown は一般的な Markdown 記法の一部を Slack mrkdwn に変換します。
//
// コードブロックの中身は変換しません。原文を見せるためのブロックで - を • に
// 書き換えては、貼った本人が見たい原文が壊れます。エスケープだけはブロック内でも必要
// なので分けて適用し、そこでは escapeMrkdwn ではなく無条件のエスケープを使います
// （原文を見せる場では <https://…> の形をした文字列もただの文字列だからです）。
func formatMarkdown(message string) string {
	return replaceSegments(message, fencedBlockRegex, convertMarkdown, mrkdwnEscaper.Replace)
}

// replaceSegments は re にマッチする区間とその外側に、別々の変換を適用して連結します。
// 「一部の区間だけ扱いを変える」処理がこのファイルに 2 つあり（コードブロックの内外と、
// リンク・メンションの内外）、変わるのは正規表現と変換だけで走査の形は同じです。
func replaceSegments(s string, re *regexp.Regexp, outside, inside func(string) string) string {
	var sb strings.Builder
	last := 0

	for _, loc := range re.FindAllStringIndex(s, -1) {
		sb.WriteString(outside(s[last:loc[0]]))
		sb.WriteString(inside(s[loc[0]:loc[1]]))
		last = loc[1]
	}
	sb.WriteString(outside(s[last:]))

	return sb.String()
}

// keepVerbatim は区間を変換せずそのまま返します。
func keepVerbatim(s string) string { return s }

// convertMarkdown はコードブロックの外側 1 区間を mrkdwn へ変換します。
//
// リンク変換が先、エスケープが後です。Slack が & < > のエスケープを求めるのはプレーン
// テキストだけで、<URL|表示テキスト> の内側は構文として解釈済みだからです。逆順にすると
// 署名付き URL のクエリ区切り & が &amp; に化けて署名が変わり、URL が壊れます。
func convertMarkdown(segment string) string {
	segment = linkRegex.ReplaceAllString(segment, "<$2|$1>")
	segment = escapeMrkdwn(segment)
	segment = boldRegex.ReplaceAllString(segment, "*$1*")
	segment = headerRegex.ReplaceAllString(segment, "*$1*")
	return listItemRegex.ReplaceAllString(segment, "• ")
}

// escapeMrkdwn は、プレーンテキスト中の Slack 制御文字を実体参照へ変換します。リンク・
// メンションの構文と既存の実体参照は残します。> もエスケープするため行頭の引用記法は
// 使えません（引用の > とエスケープが必要な > を区別できないためです）。
func escapeMrkdwn(message string) string {
	return replaceSegments(message, preservedRegex, mrkdwnEscaper.Replace, keepVerbatim)
}

// truncateHeaderText は見出しを Slack ヘッダーブロックの上限に収めます。超えたまま送ると
// Slack が invalid_blocks を返し、通知が丸ごと届かないためです。
func truncateHeaderText(ctx context.Context, headerText string) string {
	textLen := utf8.RuneCountInString(headerText)
	if textLen <= maxHeaderLength {
		return headerText
	}

	slog.WarnContext(ctx, "The notification title is too long, truncating.",
		"current_runes", textLen,
		"max_runes", maxHeaderLength)
	return truncateWithSuffix(headerText, maxHeaderLength, headerTruncationSuffix)
}

// truncateSectionText は Slack セクションブロックの上限に収まるよう本文を短縮します。
func truncateSectionText(ctx context.Context, message string) string {
	textLen := utf8.RuneCountInString(message)
	if textLen <= maxSectionLength {
		return message
	}

	slog.WarnContext(ctx, "The notification message is too long, truncating.",
		"current_runes", textLen,
		"max_runes", maxSectionLength)
	return closeUnterminatedFence(truncateWithSuffix(message, maxSectionLength, truncationSuffix))
}

// truncateWithSuffix は s を maxLen 文字以内に収め、末尾に suffix を付けます。
// 呼び出し側が上限超過を判定済みであることが前提です。
//
// 判定はルーン数、切り詰めは書記素クラスタ単位です（理由は truncateGraphemes）。
// クラスタ数はルーン数以下なので、ルーン判定が短縮の必要な場合を取りこぼすことはありません。
func truncateWithSuffix(s string, maxLen int, suffix string) string {
	return truncateGraphemes(s, maxLen-utf8.RuneCountInString(suffix), suffix)
}

// closeUnterminatedFence は、切り詰めでコードブロックが途中で切れた場合に閉じフェンスを
// 補います。長い出力を貼る Body.Block が最も切り詰めに当たりやすく、閉じフェンスごと落ちると
// 以降の描画が崩れるためです。
func closeUnterminatedFence(message string) string {
	if strings.Count(message, codeFence)%2 == 0 {
		return message
	}

	return message + "\n" + codeFence
}

// buildFooterBlock は送信時刻を表示する Slack コンテキストブロックを構築します。
func buildFooterBlock() block {
	return contextBlock("notification-context", "送信時刻: "+jst.FormatTimestamp(jst.Now()))
}
