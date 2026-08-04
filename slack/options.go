package slack

// Option は通知の表示設定をカスタマイズするための関数シグネチャです。
//
// 対象の型は非公開です。呼び出し側は本ファイルの With 系関数を通してのみ
// 設定でき、内部構造には触れません。
//
// # 効かない場合について
//
// Slack アプリ経由で作成した Incoming Webhook は、投稿先チャンネル・表示名・
// アイコンを常にアプリの設定から取り、ペイロードでの上書きを無視します。
// 本ファイルのオプションが効くのは旧来の custom integration 版の Webhook だけです。
// サービスごとに表示を変えたい場合は Slack アプリを分けてください。
//
// いずれも既定値を持ちません。指定しなければ、そのフィールドは送信されません。
type Option func(*notifier)

// WithUsername は投稿者の表示名を設定します。
func WithUsername(username string) Option {
	return func(n *notifier) { n.username = username }
}

// WithIconEmoji は投稿者のアイコン絵文字を設定します。
func WithIconEmoji(emoji string) Option {
	return func(n *notifier) { n.iconEmoji = emoji }
}

// WithChannel は投稿先チャンネルを設定します。
func WithChannel(channel string) Option {
	return func(n *notifier) { n.channel = channel }
}
