package notify

import (
	"fmt"
	"strings"
)

// codeFence は Block が使うコードブロックのフェンスです。
const codeFence = "```"

// Body は通知本文を組み立てるビルダーです。
//
// 各メソッドは値が空の場合に何も書き込まないため、呼び出し側は
// 項目ごとに存在チェックを書く必要がありません。ゼロ値から利用でき、
// すべてのメソッドはチェーン可能です。
//
// 出力は特定のチャネルに依存しない標準的な Markdown です
// （太字は **強調**、リンクは [表示テキスト](URL)）。
// Slack 固有の mrkdwn への変換は slack パッケージが行うため、
// 新しいチャネルを追加しても本文の組み立てコードは変更不要です。
type Body struct {
	sb strings.Builder
}

// NewBody は空の Body を返します。
func NewBody() *Body { return &Body{} }

// Text は任意の 1 行を追記します。s が空の場合は何もしません。
func (b *Body) Text(s string) *Body {
	if s == "" {
		return b
	}
	b.writeLine(s)
	return b
}

// Field は「**ラベル:** 値」の 1 行を追記します。値が空の場合は何もしません。
func (b *Body) Field(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, value))
	return b
}

// Code は「**ラベル:** `値`」の 1 行を追記します。値が空の場合は何もしません。
// 識別子やコマンド名など、等幅で表示したい短い値に使用します。
//
// 等幅の値に単位や絵文字を添えたい、1 行に複数の等幅の値を並べたいなど、
// 「ラベル + 単一の値」に収まらない場合は Mono と Field を組み合わせてください。
func (b *Body) Code(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, Mono(value)))
	return b
}

// Link は「**ラベル:** [表示テキスト](URL)」の 1 行を追記します。
// url が空の場合は何もしません。text が空の場合は url を表示テキストにします。
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

// LinkOrField は、url があればリンクとして、無ければ text をそのまま値として追記します。
// url と text がどちらも空の場合は何もしません。
//
// リンク先が任意な項目のための入口です。ストレージ上の成果物のように、
// 参照先 URL を作れることもあれば URI しか手元に無いこともある項目で、
// 呼び出し側が毎回同じ分岐を書くのを避けます。
func (b *Body) LinkOrField(label, url, text string) *Body {
	if url == "" {
		return b.Field(label, text)
	}
	return b.Link(label, url, text)
}

// Error は「**ラベル:**」に続けてエラー内容を追記します。
// err が nil の場合は NotAvailable を表示します。
// 既に本文がある場合は 1 行空けてから追記します。
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

// Block は「**ラベル:**」に続けてコードブロックを追記します。
// content が空の場合は NotAvailable を表示します。
// 既に本文がある場合は 1 行空けてから追記します。
//
// content 中のバックティックは ' に置き換えます。エラー文字列などに
// バックティックが含まれるとコードブロックがそこで閉じてしまうためです。
//
// フェンスは開始・終了とも独立した行に置きます。Markdown ではフェンスの
// 開始行の残りが言語指定として解釈されるため、```内容``` と 1 行に詰めると
// content の 1 行目が言語名として食われ、終了フェンスも行頭に無いので
// ブロックが閉じません。
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

// Empty は本文がまだ 1 行も書き込まれていないかどうかを返します。
func (b *Body) Empty() bool { return b.sb.Len() == 0 }

// String は組み立てた本文を返します。
// 1 行も書き込まれていない場合は NotAvailable を返します。
// 本文が空の通知は意図された結果ではなく、Slack のセクションブロックも
// 空文字列を受け付けないためです。
func (b *Body) String() string {
	if b.Empty() {
		return NotAvailable
	}
	return strings.TrimRight(b.sb.String(), "\n")
}

// writeLine は 1 行を改行付きで書き込みます。
func (b *Body) writeLine(s string) {
	b.sb.WriteString(s)
	b.sb.WriteString("\n")
}

// separate は既に本文がある場合に空行を挿入します。
func (b *Body) separate() {
	if !b.Empty() {
		b.sb.WriteString("\n")
	}
}

// Mono は s を等幅表示の記法で包んだ文字列を返します。s が空の場合は空文字を返すため、
// そのまま Field へ渡せば行ごと省かれます。
//
// Body のメソッドは「ラベル + 単一の値」しか組み立てられないため、単位や絵文字を
// 添えたい呼び出し側がバックティックを直書きしがちです。しかし本文の Markdown 記法を
// 知るのは本パッケージだけ、というのがチャネル非依存を成り立たせている境界なので、
// 記法を書く必要がある場合の出口をここに用意します。
//
//	body.Field("Seed", notify.Mono(strconv.Itoa(seed))+" 🎲")
//	body.Field("ブランチ", notify.Mono(base)+" ← "+notify.Mono(feature))
func Mono(s string) string {
	if s == "" {
		return ""
	}
	return "`" + sanitizeBacktick(s) + "`"
}

// sanitizeBacktick は等幅表示を壊すバックティックを ' に置き換えます。
func sanitizeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}
