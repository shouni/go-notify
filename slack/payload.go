package slack

// このファイルは、Incoming Webhook に送る JSON の形を持ちます。
//
// 以前は slack-go/slack の構造体を使っていましたが、使っていたのは JSON タグ付きの
// 構造体 5 種と、そのコンストラクタだけでした。API クライアントも Socket Mode も
// 使わないのに、go-notify を使う全サービスの依存グラフに slack-go と gorilla/websocket
// が載り、bump と脆弱性対応の対象になっていました。使うブロックは header / divider /
// section / context の 4 種で、どれも数年変わっていない基本形なので、形をここで持ちます。
//
// 出力の JSON は slack-go 時代と同じです（ゴールデンテストで固定）。唯一の差は、
// あちらが omitempty 無しで常に出していた "replace_original":false と
// "delete_original":false が消えたことで、Webhook ではどちらも意味を持ちません。

// webhookMessage は Incoming Webhook のペイロードです。
type webhookMessage struct {
	// Text は、blocks がトップレベルにあるときはプッシュ通知などのフォールバックです。
	Text        string       `json:"text,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
	Blocks      []block      `json:"blocks,omitempty"`
}

// attachment は色帯つきの囲みです。Blocks はこの中に入ります。
type attachment struct {
	Color    string  `json:"color,omitempty"`
	Fallback string  `json:"fallback,omitempty"`
	Blocks   []block `json:"blocks,omitempty"`
}

// block は Block Kit のブロック 1 つです。種類ごとに使うフィールドが違い、
// 使わないものは omitempty で消えます（JSON の形は slack-go の各ブロック型と同じ）。
type block struct {
	Type     string       `json:"type"`
	BlockID  string       `json:"block_id,omitempty"`
	Text     *textObject  `json:"text,omitempty"`
	Elements []textObject `json:"elements,omitempty"`
}

// textObject は plain_text または mrkdwn のテキストです。
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

// headerBlock は見出しです。plain_text しか受け付けません。
func headerBlock(text string) block {
	return block{Type: blockTypeHeader, Text: &textObject{Type: textTypePlain, Text: text, Emoji: true}}
}

// dividerBlock は区切り線です。
func dividerBlock() block {
	return block{Type: blockTypeDivider}
}

// sectionBlock は本文 1 段落です。
func sectionBlock(mrkdwn string) block {
	return block{Type: blockTypeSection, Text: &textObject{Type: textTypeMrkdwn, Text: mrkdwn}}
}

// contextBlock は小さな補足行です。
func contextBlock(blockID, mrkdwn string) block {
	return block{Type: blockTypeContext, BlockID: blockID, Elements: []textObject{{Type: textTypeMrkdwn, Text: mrkdwn}}}
}
