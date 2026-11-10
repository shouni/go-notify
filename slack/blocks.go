// Package slack provides a Slack Incoming Webhook implementation of
// notify.Notifier.
//
// It converts the body's standard Markdown to Slack mrkdwn, assembles it
// into Block Kit, and posts it. NewNotifier is the only entry point; the
// posting itself is not exported.
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
	// maxHeaderLength is the maximum length of a Slack header block. It is
	// Slack's own limit.
	maxHeaderLength = 150
	// maxSectionBlocks is the maximum number of section blocks the body is
	// split into. Slack allows 50 blocks per message, and the heading,
	// divider and footer take 3. It also caps what is readable as a
	// notification, so it stays well below that.
	maxSectionBlocks = 20
	// maxSectionLength is the maximum length of a Slack section block, with
	// a margin under Slack's limit of 3000.
	maxSectionLength = 2900
	// headerTruncationSuffix is appended when the heading is truncated.
	headerTruncationSuffix = "…"
	// truncationSuffix is appended when the body is truncated. It is
	// Japanese on purpose: it appears in Slack for the owner.
	truncationSuffix = "\n\n... (メッセージが長すぎるため省略されました)"
	// codeFence is the code block fence in the body. It must match the
	// sequence notify.Body writes (changing only one side breaks detection).
	codeFence = "```"
)

var (
	// boldRegex converts Markdown bold to Slack mrkdwn bold.
	boldRegex = regexp.MustCompile(`\*\*(.*?)\*\*`) // **text** -> *text*
	// headerRegex converts a Markdown heading to Slack mrkdwn bold.
	headerRegex = regexp.MustCompile(`(?m)^##\s*(.*)$`) // ## Title -> *Title*
	// listItemRegex converts a Markdown list item to a bullet for Slack
	// mrkdwn. It matches whitespace on the same line only (\s would swallow
	// the newline too, replacing the line break before it along with the
	// marker).
	listItemRegex = regexp.MustCompile(`(?m)^[ \t]*-[ \t]+`) // - item -> • item
	// linkRegex converts a Markdown link to a Slack mrkdwn link.
	//
	// The URL may contain one level of balanced parentheses. Ending it at
	// the first ")" cuts a URL such as a_(b)_c midway and yields a broken
	// link mixed with the display text. A URL with unbalanced parentheses,
	// or a link whose display text contains ], does not match and stays as
	// Markdown (literal Markdown reads better than a broken link).
	linkRegex = regexp.MustCompile(`\[([^\]]*)\]\(((?:[^()\s]|\([^()\s]*\))+)\)`) // [text](url) -> <url|text>
	// fencedBlockRegex matches a code block opened and closed by fences at
	// the start of a line. Without a closing fence it does not match and the
	// text is converted as usual (so one broken fence cannot exempt the rest
	// of the body from conversion).
	fencedBlockRegex = regexp.MustCompile("(?ms)^" + codeFence + "[^\n]*\n.*?^" + codeFence + "[ \t]*$")
	// preservedRegex matches the parts that must not be escaped.
	//
	// These are the <...> that Slack parses as syntax, namely links with a
	// scheme (<https://…|text>, <mailto:…>) and mentions (<@U123>, <#C123>,
	// <!here>), plus entities that are already escaped.
	//
	// It does not keep every <...> as is, because that would mistake plain
	// angle brackets such as <nil> in an error string for link syntax. Only
	// those starting with a scheme or a mention sigil count as syntax.
	preservedRegex = regexp.MustCompile(`<(?:[a-zA-Z][a-zA-Z0-9+.\-]*:[^<>]*|[@#!][^<>]*)>|&(?:amp|lt|gt);`)
)

// mrkdwnEscaper replaces the three characters Slack treats specially with
// entities. strings.Replacer does not rescan its output, so & is never
// escaped twice.
var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// buildMessageBlocks builds the Slack Block Kit blocks. The heading and the
// body are cut down to fit their blocks' limits.
func buildMessageBlocks(ctx context.Context, headerText string, message string) ([]block, error) {
	if headerText == "" {
		return nil, errors.New("notification title is empty")
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

// buildSectionTexts converts the body to mrkdwn strings for Slack section
// blocks, split at line boundaries so each fits one block's limit. An empty
// body yields none.
//
// It splits rather than truncates because a cut inside a link breaks the
// markup (a signed URL is 860 characters, and three of them alone crossed
// the limit and broke the last link). Only when the split still exceeds
// maxSectionBlocks is the last block truncated.
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
	// The last block is within the limit, so make room for the truncation
	// note and always append it. The fence is closed before the note (so the
	// note does not end up inside the code block).
	room := maxSectionLength - utf8.RuneCountInString(truncationSuffix) - fenceReserve
	*last = closeUnterminatedFence(truncateGraphemes(*last, room, "")) + truncationSuffix
	return chunks
}

// fenceReserve is the number of characters kept free for the closing fence
// when splitting inside a code block.
const fenceReserve = len("\n" + codeFence)

// maxPieceLength is the maximum length of one piece when a line is split.
//
// A block split inside a fence becomes "opening fence + newline + text +
// newline + closing fence". Subtracting only the closing side
// (fenceReserve) from the budget is not enough: the reopening fence and the
// newline after it count too. Getting this wrong by even one character
// sends an oversized block, Slack answers invalid_blocks, and the whole
// notification is lost.
const maxPieceLength = maxSectionLength - 2*fenceReserve

// splitSectionText splits a mrkdwn body at line boundaries so that each
// element is at most maxSectionLength. When it splits inside a code block,
// it closes the previous element with a closing fence and starts the next
// with an opening fence. A line over the limit is split at spaces.
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

// isFenceLine reports whether line is a code block's opening or closing
// fence.
func isFenceLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), codeFence)
}

// splitLongLine splits a line longer than maxLen.
//
// It looks for a cut at a space first, then anywhere outside a link.
// Spaces come first so words are not cut, but **Japanese puts no spaces
// between words**, so a line without one is the normal case, not an
// exception. Giving up there would send an oversized block, Slack would
// answer invalid_blocks, and the whole notification would be lost (the same
// failure as an untruncated heading).
//
// It avoids the inside of link syntax <...>, but when the whole window is
// inside a link it cuts the link anyway. One broken link is a lighter
// failure than losing the whole notification to an oversized block (a
// signed URL is about 900 characters, so only abnormal input gets here).
func splitLongLine(line string, maxLen int) []string {
	if utf8.RuneCountInString(line) <= maxLen {
		return []string{line}
	}

	var pieces []string
	for utf8.RuneCountInString(line) > maxLen {
		cut := lastBreakableSpace(line, maxLen)
		if cut <= 0 {
			// No space, so cut at a cluster boundary. Prefer outside a
			// link; if the whole window is inside one, cut the link.
			outside, anywhere := lastBreaks(line, maxLen)
			if cut = outside; cut <= 0 {
				cut = anywhere
			}
			if cut <= 0 {
				break // A single cluster exceeds the limit; it cannot be split further.
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

// lastBreaks returns two of the last grapheme cluster boundaries within the
// first maxLen characters: outside is the last one outside link syntax
// <...> (-1 if none), and anywhere is the last one regardless of position.
// Both are byte offsets.
//
// It chooses cluster boundaries so that combining characters and ZWJ emoji
// are not split (the same reason as truncateGraphemes). It counts runes:
// the limit is checked with utf8.RuneCountInString, so counting clusters
// here alone would overshoot by one on input where a cluster is several
// runes (combining sequences or invalid UTF-8).
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

// lastBreakableSpace returns the byte offset of the last space within the
// first maxLen characters, ignoring spaces inside <...>. It returns -1 if
// there is none.
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

// formatMarkdown converts a subset of common Markdown to Slack mrkdwn.
//
// Code block contents are not converted. Rewriting - to • in a block meant
// to show text verbatim would break the very text its author wants to see.
// Escaping is still needed inside blocks, so it is applied separately, and
// there it is unconditional rather than escapeMrkdwn (where text is shown
// verbatim, a string shaped like <https://…> is just a string).
func formatMarkdown(message string) string {
	return replaceSegments(message, fencedBlockRegex, convertMarkdown, mrkdwnEscaper.Replace)
}

// replaceSegments applies one transform to the spans matching re and
// another to the text outside them, and joins the result. This file has
// two "treat some spans differently" passes (inside and outside code
// blocks, inside and outside links and mentions); only the regex and the
// transforms differ, and the traversal is the same.
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

// keepVerbatim returns the span unchanged.
func keepVerbatim(s string) string { return s }

// convertMarkdown converts one span outside code blocks to mrkdwn.
//
// Links are converted first and escaping comes after. Slack requires & < >
// to be escaped in plain text only; inside <URL|text> they are already
// parsed as syntax. In the reverse order, a signed URL's & query
// separators turn into &amp;, the signature changes, and the URL breaks.
func convertMarkdown(segment string) string {
	segment = linkRegex.ReplaceAllString(segment, "<$2|$1>")
	segment = escapeMrkdwn(segment)
	segment = boldRegex.ReplaceAllString(segment, "*$1*")
	segment = headerRegex.ReplaceAllString(segment, "*$1*")
	return listItemRegex.ReplaceAllString(segment, "• ")
}

// escapeMrkdwn converts Slack control characters in plain text to entities,
// keeping link and mention syntax and existing entities. Because > is
// escaped too, a blockquote at the start of a line is unsupported (a quote's
// > cannot be told apart from a > that needs escaping).
func escapeMrkdwn(message string) string {
	return replaceSegments(message, preservedRegex, mrkdwnEscaper.Replace, keepVerbatim)
}

// truncateHeaderText fits the heading within the Slack header block limit.
// Sent over the limit, Slack answers invalid_blocks and the whole
// notification is lost.
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

// truncateSectionText shortens the body to fit the Slack section block
// limit.
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

// truncateWithSuffix fits s within maxLen characters and appends suffix. It
// assumes the caller has already found s to be over the limit.
//
// The check counts runes and the cut is by grapheme cluster (see
// truncateGraphemes for why). A cluster count never exceeds the rune count,
// so the rune check never misses a case that needs shortening.
func truncateWithSuffix(s string, maxLen int, suffix string) string {
	return truncateGraphemes(s, maxLen-utf8.RuneCountInString(suffix), suffix)
}

// closeUnterminatedFence adds a closing fence when truncation cut a code
// block midway. Body.Block, which carries long pasted output, is the most
// likely to be truncated, and losing its closing fence breaks the rendering
// of everything after it.
func closeUnterminatedFence(message string) string {
	if strings.Count(message, codeFence)%2 == 0 {
		return message
	}

	return message + "\n" + codeFence
}

// buildFooterBlock builds the Slack context block showing the send time.
// Its 「送信時刻」 label is Japanese on purpose: it appears in Slack for the
// owner.
func buildFooterBlock() block {
	return contextBlock("notification-context", "送信時刻: "+jst.FormatTimestamp(jst.Now()))
}
