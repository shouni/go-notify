// Package slack は、Slack Webhook へのメッセージ投稿と Block Kit 形式への
// 整形を行うクライアントを提供します。
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
	// maxSectionLength は Slack セクションブロックの最大文字数です。
	maxSectionLength = 2900
	// truncationSuffix はメッセージ切り捨て時に追加するサフィックスです。
	truncationSuffix = "\n\n... (メッセージが長すぎるため省略されました)"
)

var (
	// boldRegex は Markdown の太字記法を Slack mrkdwn の太字記法に変換します。
	boldRegex = regexp.MustCompile(`\*\*(.*?)\*\*`) // **text** -> *text*
	// headerRegex は Markdown の見出しを Slack mrkdwn の太字記法に変換します。
	headerRegex = regexp.MustCompile(`(?m)^##\s*(.*)$`) // ## Title -> *Title*
	// listItemRegex は Markdown のリスト項目を Slack mrkdwn 向けの箇条書きに変換します。
	listItemRegex = regexp.MustCompile(`(?m)^\s*-\s+`) // - item -> • item
	// linkRegex は Markdown のリンクを Slack mrkdwn のリンクに変換します。
	// 表示テキストに ] を、URL に ) を含むリンクは対象外です（Slack 側で
	// エスケープできないため、変換しても壊れたリンクにしかなりません）。
	linkRegex = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`) // [text](url) -> <url|text>
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
func buildMessageBlocks(headerText string, message string) ([]slack.Block, error) {
	if headerText == "" {
		return nil, errors.New("headerText は必須です")
	}

	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", headerText, true, false),
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
// リンク変換を先に、エスケープを後に行います。Slack が & < > の
// エスケープを求めるのはプレーンテキストだけで、<URL|表示テキスト> の
// 内側は構文として解釈済みのため対象外だからです。逆順にすると
// GCS の署名付き URL のクエリ区切り & が &amp; に化けて URL 自体が壊れます。
//
// 既に <URL|表示テキスト> 形式やメンションで書かれた mrkdwn はそのまま通ります。
// 行頭の > による引用記法は使えません（エスケープ対象の文字と区別できないため）。
func formatMarkdown(message string) string {
	message = linkRegex.ReplaceAllString(message, "<$2|$1>")
	message = escapeMrkdwn(message)
	message = boldRegex.ReplaceAllString(message, "*$1*")
	message = headerRegex.ReplaceAllString(message, "*$1*")
	return listItemRegex.ReplaceAllString(message, "• ")
}

// escapeMrkdwn は、プレーンテキスト中の Slack 制御文字を実体参照へ変換します。
// リンク・メンションの構文と既存の実体参照はそのまま残します。
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

// truncateSectionText は Slack セクションブロックの上限に収まるよう本文を短縮します。
func truncateSectionText(message string) string {
	textLen := utf8.RuneCountInString(message)
	if textLen <= maxSectionLength {
		return message
	}

	slog.Warn("The notification message is too long, truncating.",
		"current_runes", textLen,
		"max_runes", maxSectionLength)
	return truncateWithSuffixLimit(message, maxSectionLength, truncationSuffix)
}

// buildFooterBlock は送信時刻を表示する Slack コンテキストブロックを構築します。
func buildFooterBlock() *slack.ContextBlock {
	return slack.NewContextBlock(
		"notification-context",
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("送信時刻: %s",
			jst.Now().Format(jst.LayoutTimestamp)), false, false),
	)
}
