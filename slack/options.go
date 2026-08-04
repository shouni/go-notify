package slack

// Option は Client の設定をカスタマイズするための関数シグネチャです。
type Option func(*Client)

// WithUsername は送信ユーザー名を設定します。
func WithUsername(username string) Option {
	return func(c *Client) { c.username = username }
}

// WithIconEmoji はアイコン絵文字を設定します。
func WithIconEmoji(emoji string) Option {
	return func(c *Client) { c.iconEmoji = emoji }
}

// WithChannel はデフォルトの送信先チャンネルを設定します。
func WithChannel(channel string) Option {
	return func(c *Client) { c.channel = channel }
}
