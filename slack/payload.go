package slack

// This file holds the shape of the JSON sent to an Incoming Webhook.
//
// It used to come from slack-go/slack's structs, but all that was used were
// five JSON-tagged structs and their constructors. Neither the API client
// nor Socket Mode was used, yet slack-go and gorilla/websocket sat in the
// dependency graph of every service using go-notify, and in their bump and
// vulnerability queues. The blocks used are header / divider / section /
// context, all basic shapes unchanged for years, so the shape lives here.
//
// The JSON output is the same as in the slack-go days (pinned by a golden
// test). The only difference is that "replace_original":false and
// "delete_original":false, which slack-go always emitted without omitempty,
// are gone; neither means anything on a webhook.

// webhookMessage is the Incoming Webhook payload.
type webhookMessage struct {
	// Text is the fallback for push notifications and the like when the
	// blocks are top-level.
	Text        string       `json:"text,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
	Blocks      []block      `json:"blocks,omitempty"`
}

// attachment is a container with a colour bar. Blocks go inside it.
type attachment struct {
	Color    string  `json:"color,omitempty"`
	Fallback string  `json:"fallback,omitempty"`
	Blocks   []block `json:"blocks,omitempty"`
}

// block is a single Block Kit block. Each type uses different fields, and
// unused ones disappear through omitempty (the JSON shape matches each of
// slack-go's block types).
type block struct {
	Type     string       `json:"type"`
	BlockID  string       `json:"block_id,omitempty"`
	Text     *textObject  `json:"text,omitempty"`
	Elements []textObject `json:"elements,omitempty"`
}

// textObject is plain_text or mrkdwn text.
type textObject struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

const (
	blockTypeHeader  = "header"
	blockTypeDivider = "divider"
	blockTypeSection = "section"
	blockTypeContext = "context"

	textTypePlain  = "plain_text"
	textTypeMrkdwn = "mrkdwn"
)

// headerBlock is a heading. It accepts plain_text only.
func headerBlock(text string) block {
	return block{Type: blockTypeHeader, Text: &textObject{Type: textTypePlain, Text: text, Emoji: true}}
}

// dividerBlock is a divider line.
func dividerBlock() block {
	return block{Type: blockTypeDivider}
}

// sectionBlock is one paragraph of the body.
func sectionBlock(mrkdwn string) block {
	return block{Type: blockTypeSection, Text: &textObject{Type: textTypeMrkdwn, Text: mrkdwn}}
}

// contextBlock is a small supplementary line.
func contextBlock(blockID, mrkdwn string) block {
	return block{Type: blockTypeContext, BlockID: blockID, Elements: []textObject{{Type: textTypeMrkdwn, Text: mrkdwn}}}
}
