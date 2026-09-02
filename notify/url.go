package notify

import (
	"net/url"
	"slices"
)

// JoinURL は、ベース URL にパス要素を連結した URL を返します。base か elems のいずれかが
// 空、または base を URL として解釈できない場合は空文字列です。
//
// エラーではなく空文字列を返すのは、Body.Link と LinkOrField が「URL が空なら行ごと省く」
// 契約を持っているためで、呼び出し側はリンク行を組むかどうかの分岐を書かずに済みます。
// パス要素は url.JoinPath がエスケープしますが、スラッシュは区切りとして残ります。
//
//	body.Link("History Detail", notify.JoinURL(serviceURL, "/web/history", jobID), jobID)
func JoinURL(base string, elems ...string) string {
	if base == "" {
		return ""
	}
	if slices.Contains(elems, "") {
		return ""
	}

	joined, err := url.JoinPath(base, elems...)
	if err != nil {
		return ""
	}
	return joined
}
