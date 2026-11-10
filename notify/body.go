package notify

import (
	"fmt"
	"strings"
)

// codeFence is the code block fence that Block writes. Channels detect code
// blocks by this sequence, so changing it means changing them too.
const codeFence = "```"

// Body builds a notification body.
//
// It writes nothing for an empty value, so callers need no per-field
// presence checks. The zero value is ready to use and every method chains.
//
// The output is standard, channel-independent Markdown (**bold** for bold,
// [text](URL) for links). The slack package converts it to Slack's own
// mrkdwn, so adding a new channel needs no change to the code that builds
// bodies.
type Body struct {
	sb strings.Builder
}

// NewBody returns an empty Body.
func NewBody() *Body { return &Body{} }

// Text appends an arbitrary line.
func (b *Body) Text(s string) *Body {
	if s == "" {
		return b
	}
	b.writeLine(s)
	return b
}

// Heading appends a sub-heading line, preceded by a blank line when the body
// already has content. It is for bodies with many fields that read better
// split into groups.
func (b *Body) Heading(s string) *Body {
	if s == "" {
		return b
	}
	b.separate()
	b.writeLine("## " + s)
	return b
}

// Bullet appends a list item line. For a variable number of entries (the
// names of processed files, say) it reads better than repeated Fields.
func (b *Body) Bullet(s string) *Body {
	if s == "" {
		return b
	}
	b.writeLine("- " + s)
	return b
}

// Field appends a "**label:** value" line.
func (b *Body) Field(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, value))
	return b
}

// Code appends a "**label:** `value`" line. Use it for short values that
// should be monospaced, such as identifiers or command names. Build lines
// that do not fit this shape from CodeSpan and Field.
func (b *Body) Code(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, CodeSpan(value)))
	return b
}

// Link appends a "**label:** [text](URL)" line. When text is empty, url is
// used as the display text.
func (b *Body) Link(label, url, text string) *Body {
	if url == "" {
		return b
	}
	if text == "" {
		text = url
	}
	b.writeLine(fmt.Sprintf("**%s:** [%s](%s)", label, text, url))
	return b
}

// LinkOrField appends a link when url is set, and otherwise text as a plain
// value.
//
// For fields such as stored artifacts, where sometimes a URL can be built
// and sometimes only a URI exists, it saves callers writing the same branch
// every time.
func (b *Body) LinkOrField(label, url, text string) *Body {
	if url == "" {
		return b.Field(label, text)
	}
	return b.Link(label, url, text)
}

// URIField appends a storage URI line.
//
// A gs:// URI becomes a link to Cloud Console while its display text stays
// the gs:// string: without the link it cannot be clicked, and with the
// Console URL as display text it cannot be copied into gcloud. Anything
// other than gs:// (an http(s) input source, say) is written as a plain
// value.
func (b *Body) URIField(label, uri string) *Body {
	uri = strings.TrimSpace(uri)
	return b.LinkOrField(label, gcsConsoleURL(uri), uri)
}

// gcsConsoleURL returns the Cloud Console URL for a gs:// URI, or an empty
// string for anything else (http(s) or empty).
//
// A trailing slash is treated as a directory and goes to the bucket
// browser; a single object goes to its details page.
func gcsConsoleURL(uri string) string {
	objectPath, ok := strings.CutPrefix(uri, "gs://")
	if !ok || objectPath == "" {
		return ""
	}

	if strings.HasSuffix(objectPath, "/") {
		return "https://console.cloud.google.com/storage/browser/" + objectPath
	}
	return "https://console.cloud.google.com/storage/browser/_details/" + objectPath
}

// Error appends "**label:**" followed by the error text. A nil err writes
// NotAvailable, and a blank line goes first when the body already has
// content.
func (b *Body) Error(label string, err error) *Body {
	detail := NotAvailable
	if err != nil {
		detail = err.Error()
	}
	b.separate()
	b.writeLine(fmt.Sprintf("**%s:**", label))
	b.writeLine(detail)
	return b
}

// Block appends "**label:**" followed by a code block. Empty content writes
// NotAvailable, and a blank line goes first when the body already has
// content. Backticks in content are replaced with ' (one would close the
// code block there).
//
// Both fences go on lines of their own. On a single line the rest of the
// opening line is eaten as the info string, and the closing fence is not
// at the start of a line, so the block never closes.
func (b *Body) Block(label, content string) *Body {
	if content == "" {
		content = NotAvailable
	}
	b.separate()
	b.writeLine(fmt.Sprintf("**%s:**", label))
	b.writeLine(codeFence)
	b.writeLine(sanitizeBacktick(content))
	b.writeLine(codeFence)
	return b
}

// Empty reports whether nothing has been written to the body yet.
func (b *Body) Empty() bool { return b.sb.Len() == 0 }

// String returns the built body, or NotAvailable when nothing was written
// (an empty notification is never the intent, and Slack's section block
// rejects empty text).
func (b *Body) String() string {
	if b.Empty() {
		return NotAvailable
	}
	return strings.TrimRight(b.sb.String(), "\n")
}

// clone returns a separate Body with the same content. It copies the raw
// buffer because rebuilding from String() drops the trailing newline, and
// separate() could then no longer insert its blank line.
func (b *Body) clone() *Body {
	c := NewBody()
	c.sb.WriteString(b.sb.String())
	return c
}

// writeLine writes one line followed by a newline.
func (b *Body) writeLine(s string) {
	b.sb.WriteString(s)
	b.sb.WriteString("\n")
}

// separate inserts a blank line when the body already has content.
func (b *Body) separate() {
	if !b.Empty() {
		b.sb.WriteString("\n")
	}
}

// CodeSpan returns s wrapped as a code span. An empty s returns an empty
// string, so passing the result straight to Field omits the whole line.
//
// Code builds only "label + one value", so callers wanting a unit or an
// emoji alongside tend to write backticks by hand. Channel independence
// rests on this package being the only one that knows Markdown, so the
// way out for callers that need the markup is here.
//
//	body.Field("Seed", notify.CodeSpan(strconv.Itoa(seed))+" 🎲")
//	body.Field("Branch", notify.CodeSpan(base)+" ← "+notify.CodeSpan(feature))
func CodeSpan(s string) string {
	if s == "" {
		return ""
	}
	return "`" + sanitizeBacktick(s) + "`"
}

// sanitizeBacktick replaces backticks, which would break monospaced
// display, with '.
func sanitizeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}
