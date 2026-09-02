package notify

import (
	"fmt"
	"strings"
)

// codeFence は Block が使うコードブロックのフェンスです。
// 各チャネルはこの並びを目印にコードブロックを検出するため、変える場合は実装側も揃えます。
const codeFence = "```"

// Body は通知本文を組み立てるビルダーです。
//
// 値が空のときは何も書き込まないので、呼び出し側は項目ごとの存在チェックを
// 書かずに済みます。ゼロ値から使え、すべてのメソッドはチェーン可能です。
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

// Text は任意の 1 行を追記します。
func (b *Body) Text(s string) *Body {
	if s == "" {
		return b
	}
	b.writeLine(s)
	return b
}

// Heading は小見出しの 1 行を追記します。既に本文があれば 1 行空けます。
// 項目が多く、意味のまとまりで区切りたい本文のための入口です。
func (b *Body) Heading(s string) *Body {
	if s == "" {
		return b
	}
	b.separate()
	b.writeLine("## " + s)
	return b
}

// Bullet は箇条書きの 1 行を追記します。
// 件数が可変のもの（処理したファイル名など）は、Field を繰り返すより読みやすくなります。
func (b *Body) Bullet(s string) *Body {
	if s == "" {
		return b
	}
	b.writeLine("- " + s)
	return b
}

// Field は「**ラベル:** 値」の 1 行を追記します。
func (b *Body) Field(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, value))
	return b
}

// Code は「**ラベル:** `値`」の 1 行を追記します。識別子やコマンド名など、
// 等幅で表示したい短い値に使います。この形に収まらない行は CodeSpan と Field で組みます。
func (b *Body) Code(label, value string) *Body {
	if value == "" {
		return b
	}
	b.writeLine(fmt.Sprintf("**%s:** %s", label, CodeSpan(value)))
	return b
}

// Link は「**ラベル:** [表示テキスト](URL)」の 1 行を追記します。
// text が空なら url を表示テキストにします。
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

// LinkOrField は、url があればリンクとして、無ければ text を値として追記します。
//
// ストレージ上の成果物のように、参照先 URL を作れることもあれば URI しか無いことも
// ある項目で、呼び出し側が毎回同じ分岐を書くのを避けます。
func (b *Body) LinkOrField(label, url, text string) *Body {
	if url == "" {
		return b.Field(label, text)
	}
	return b.Link(label, url, text)
}

// URIField は、ストレージ URI の 1 行を追記します。
//
// gs:// は Cloud Console へのリンクにしつつ、表示は gs:// のまま残します。リンクが
// 無いとクリックできず、表示まで Console の URL にすると gcloud へコピーできないためです。
// gs:// 以外（http(s) の入力ソースなど）は素の値として並びます。
func (b *Body) URIField(label, uri string) *Body {
	uri = strings.TrimSpace(uri)
	return b.LinkOrField(label, gcsConsoleURL(uri), uri)
}

// gcsConsoleURL は gs:// URI に対応する Cloud Console の URL を返します。
// gs:// 以外（http(s) や空文字）の場合は空文字を返します。
//
// 末尾がスラッシュのものはディレクトリ扱いでバケットブラウザへ、
// 単体オブジェクトは詳細ページへ飛ばします。
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

// Error は「**ラベル:**」に続けてエラー内容を追記します。
// err が nil なら NotAvailable、既に本文があれば 1 行空けます。
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

// Block は「**ラベル:**」に続けてコードブロックを追記します。空なら NotAvailable、
// 既に本文があれば 1 行空けます。content 中のバックティックは ' に置き換えます
// （含まれているとコードブロックがそこで閉じるため）。
//
// フェンスは開始・終了とも独立した行に置きます。1 行に詰めると開始行の残りが言語指定
// として食われ、終了フェンスも行頭に無いのでブロックが閉じません。
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

// String は組み立てた本文を返します。1 行も書き込まれていなければ NotAvailable です
// （空の通知は意図された結果ではなく、Slack のセクションブロックも空文字を受け付けません）。
func (b *Body) String() string {
	if b.Empty() {
		return NotAvailable
	}
	return strings.TrimRight(b.sb.String(), "\n")
}

// clone は同じ内容を持つ別の Body を返します。内部バッファをそのまま写すのは、
// String() から組み直すと末尾の改行が落ち、separate() が空行を入れられなくなるためです。
func (b *Body) clone() *Body {
	c := NewBody()
	c.sb.WriteString(b.sb.String())
	return c
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

// CodeSpan は s をコードスパン記法で包んだ文字列を返します。s が空なら空文字なので、
// そのまま Field へ渡せば行ごと省かれます。
//
// Code は「ラベル + 単一の値」しか組めず、単位や絵文字を添えたい呼び出し側が
// バックティックを直書きしがちです。Markdown 記法を知るのは本パッケージだけ、という
// 境界がチャネル非依存を支えているので、記法が要る場合の出口をここに置きます。
//
//	body.Field("Seed", notify.CodeSpan(strconv.Itoa(seed))+" 🎲")
//	body.Field("ブランチ", notify.CodeSpan(base)+" ← "+notify.CodeSpan(feature))
func CodeSpan(s string) string {
	if s == "" {
		return ""
	}
	return "`" + sanitizeBacktick(s) + "`"
}

// sanitizeBacktick は等幅表示を壊すバックティックを ' に置き換えます。
func sanitizeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}
