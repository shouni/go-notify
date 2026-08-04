package slack

// Option は通知の表示設定をカスタマイズするための関数シグネチャです。
//
// 対象の型は非公開です。呼び出し側は本ファイルの With 系関数を通してのみ
// 設定でき、内部構造には触れません。
type Option func(*notifier)

// WithUsername は送信ユーザー名を設定します。
func WithUsername(username string) Option {
	return func(n *notifier) { n.username = username }
}

// WithIconEmoji はアイコン絵文字を設定します。
func WithIconEmoji(emoji string) Option {
	return func(n *notifier) { n.iconEmoji = emoji }
}

// WithChannel はデフォルトの送信先チャンネルを設定します。
func WithChannel(channel string) Option {
	return func(n *notifier) { n.channel = channel }
}
