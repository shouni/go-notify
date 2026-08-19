// Package slack は、notify.Notifier の Slack Incoming Webhook 実装を提供します。
//
// 本文の標準 Markdown を Slack mrkdwn へ変換し、Block Kit 形式に組み立てて投稿します。
// 入口は NewNotifier だけで、投稿処理そのものは公開していません。
package slack

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shouni/go-utils/jst"
	"github.com/slack-go/slack"
)

const (
	// maxHeaderLength は Slack ヘッダーブロックの最大文字数です。Slack の上限そのものです。
	maxHeaderLength = 150
	// maxSectionLength は Slack セクションブロックの最大文字数です。
	// Slack の上限 3000 の手前に余裕を取っています。
	maxSectionLength = 2900
	// headerTruncationSuffix は見出し切り捨て時に追加するサフィックスです。
	headerTruncationSuffix = "…"
	// truncationSuffix は本文切り捨て時に追加するサフィックスです。
	truncationSuffix = "\n\n... (メッセージが長すぎるため省略されました)"
	// codeFence は本文中のコードブロックのフェンスです。
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
func buildMessageBlocks(headerText string, message string) ([]slack.Block, error) {
	if headerText == "" {
		return nil, errors.New("通知の見出しが空です")
	}

	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", truncateHeaderText(headerText), true, false),
		),
		slack.NewDividerBlock(),
	}

	sectionText := buildSectionText(message)
	if sectionText != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", sectionText, false, false), nil, nil),
		)
	}

	blocks = append(blocks, buildFooterBlock())

	return blocks, nil
}

// buildSectionText は本文を Slack セクションブロック用の mrkdwn 文字列に変換します。
func buildSectionText(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}

	return truncateSectionText(formatMarkdown(message))
}

// formatMarkdown は一般的な Markdown 記法の一部を Slack mrkdwn に変換します。
//
// コードブロックの中身は記法変換の対象外です。原文をそのまま見せるための
// ブロックで - を • に、**text** を *text* に書き換えては、貼った本人が
// 見たいはずの原文が壊れます。
//
// エスケープだけはブロック内でも必要なので、変換と分けて適用します。
// ブロック内で escapeMrkdwn ではなく無条件のエスケープを使うのは同じ理由です。
// 原文を見せるのが目的である以上、たまたま <https://…> の形をした文字列は
// リンク構文ではなくただの文字列として扱うべきだからです。
func formatMarkdown(message string) string {
	var sb strings.Builder
	last := 0

	for _, loc := range fencedBlockRegex.FindAllStringIndex(message, -1) {
		sb.WriteString(convertMarkdown(message[last:loc[0]]))
		sb.WriteString(mrkdwnEscaper.Replace(message[loc[0]:loc[1]]))
		last = loc[1]
	}
	sb.WriteString(convertMarkdown(message[last:]))

	return sb.String()
}

// convertMarkdown はコードブロックの外側 1 区間を mrkdwn へ変換します。
//
// リンク変換を先に、エスケープを後に行います。Slack が & < > の
// エスケープを求めるのはプレーンテキストだけで、<URL|表示テキスト> の
// 内側は構文として解釈済みのため対象外だからです。逆順にすると
// 署名付き URL のクエリ区切り & が &amp; に化けて署名が変わり、URL が壊れます。
func convertMarkdown(segment string) string {
	segment = linkRegex.ReplaceAllString(segment, "<$2|$1>")
	segment = escapeMrkdwn(segment)
	segment = boldRegex.ReplaceAllString(segment, "*$1*")
	segment = headerRegex.ReplaceAllString(segment, "*$1*")
	return listItemRegex.ReplaceAllString(segment, "• ")
}

// escapeMrkdwn は、プレーンテキスト中の Slack 制御文字を実体参照へ変換します。
// リンク・メンションの構文と既存の実体参照はそのまま残します。
//
// > がここでエスケープされるため、行頭の > による引用記法は使えません
// （引用の > とエスケープが必要な > を区別できないためです）。
func escapeMrkdwn(message string) string {
	var sb strings.Builder
	last := 0

	for _, loc := range preservedRegex.FindAllStringIndex(message, -1) {
		sb.WriteString(mrkdwnEscaper.Replace(message[last:loc[0]]))
		sb.WriteString(message[loc[0]:loc[1]])
		last = loc[1]
	}
	sb.WriteString(mrkdwnEscaper.Replace(message[last:]))

	return sb.String()
}

// truncateHeaderText は見出しを Slack ヘッダーブロックの上限に収めます。
//
// 上限を超えたまま送ると Slack は invalid_blocks を返し、通知が丸ごと届きません。
// 見出しの末尾が読めることより通知そのものが届くことが重要なので、切り詰めます。
func truncateHeaderText(headerText string) string {
	textLen := utf8.RuneCountInString(headerText)
	if textLen <= maxHeaderLength {
		return headerText
	}

	slog.Warn("The notification title is too long, truncating.",
		"current_runes", textLen,
		"max_runes", maxHeaderLength)
	return truncateWithSuffix(headerText, maxHeaderLength, headerTruncationSuffix)
}

// truncateSectionText は Slack セクションブロックの上限に収まるよう本文を短縮します。
func truncateSectionText(message string) string {
	textLen := utf8.RuneCountInString(message)
	if textLen <= maxSectionLength {
		return message
	}

	slog.Warn("The notification message is too long, truncating.",
		"current_runes", textLen,
		"max_runes", maxSectionLength)
	return closeUnterminatedFence(truncateWithSuffix(message, maxSectionLength, truncationSuffix))
}

// truncateWithSuffix は s を maxLen 文字以内に収め、末尾に suffix を付けます。
// 呼び出し側が上限超過を判定済みであることを前提にします。
//
// 上限の判定はルーン数ですが、切り詰めそのものは書記素クラスタ単位です
// （truncateGraphemes の仕様）。ルーンで切ると濁点や ZWJ 絵文字が分断されるため
// 切る側はクラスタ単位でなければならず、一方クラスタ数はルーン数以下なので、
// ルーンでの判定は「短縮が必要な場合」を取りこぼしません。
func truncateWithSuffix(s string, maxLen int, suffix string) string {
	return truncateGraphemes(s, maxLen-utf8.RuneCountInString(suffix), suffix)
}

// closeUnterminatedFence は、切り詰めでコードブロックの途中が切れた場合に閉じフェンスを補います。
//
// 長い出力を貼るための Body.Block が最も切り詰めに当たりやすく、そこで切ると
// 閉じフェンスごと落ちて開いたままの mrkdwn になります。以降の描画が崩れるため、
// 切った側で閉じます。
func closeUnterminatedFence(message string) string {
	if strings.Count(message, codeFence)%2 == 0 {
		return message
	}

	return message + "\n" + codeFence
}

// buildFooterBlock は送信時刻を表示する Slack コンテキストブロックを構築します。
func buildFooterBlock() *slack.ContextBlock {
	return slack.NewContextBlock(
		"notification-context",
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("送信時刻: %s",
			jst.Now().Format(jst.LayoutTimestamp)), false, false),
	)
}
