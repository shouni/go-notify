package notify

import (
	"net/url"
	"slices"
)

// JoinURL は、ベース URL にパス要素を連結した URL を返します。
// 組み立てられない場合は空文字列を返します。
//
// 空を返すのは、base が空・elems のいずれかが空・base が URL として
// 解釈できない、のいずれかです。エラーではなく空文字列にするのは、
// Body.Link と LinkOrField が「URL が空なら行ごと省く」契約を持っているためで、
// 呼び出し側はリンク行を組み立てるかどうかの分岐を書かずに済みます。
//
// パス要素は url.JoinPath がエスケープしますが、スラッシュは区切りとして残ります。
// "/web/history" のようなパスをそのまま渡せるようにするためです。
//
//	body.Link("History Detail", notify.JoinURL(serviceURL, "/web/history", jobID), jobID)
//
// この形が本パッケージにあるのは、消費側のサービスがまったく同じ関数を
// 何本も抱えていたためです（serviceURL か ID が空なら空、url.JoinPath が
// 失敗しても空）。「空なら行を落とす」という契約を決めているのがこちらである以上、
// それに合わせて URL を組み立てる部分もこちらに置きます。
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
