# 🔔 Go Notify

[![CI](https://github.com/shouni/go-notify/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-notify/actions/workflows/ci.yml)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-notify)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-notify)](https://github.com/shouni/go-notify/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-notify.svg)](https://pkg.go.dev/github.com/shouni/go-notify)

## 🚀 概要

**Go Notify** は、非同期パイプラインの実行結果を通知するための Go ライブラリです。
CLI は持たず、アプリケーションに組み込んで使います。

通知チャネルに依存しない `notify` パッケージと、その実装である `slack` パッケージに
分かれており、**本文を組み立てるコードはチャネルを意識しません**。

### 主要な特徴

* **チャネル非依存の本文組み立て**
  `notify.Body` は標準的な Markdown を出力し、Slack 固有の mrkdwn 記法への変換は
  `slack` パッケージが担当します。新しいチャネルを追加しても呼び出し側は変わりません。
* **空値の自動スキップ**
  `Body` の各メソッドは値が空なら何も書き込みません。項目ごとの `if` が不要になります。
* **通知先未設定は「無効」であってエラーではない**
  Webhook URL が空なら `notify.Disabled()` が返り、以降の呼び出しは黙って成功します。
  通知はアプリケーションの主目的ではなく、宛先の未設定で起動を止める理由はありません。
* **堅牢な通信基盤**
  外部通信はすべて [`go-http-kit`](https://github.com/shouni/go-http-kit) の
  `httpkit.Requester` 経由で、指数バックオフ・タイムアウト・SSRF 対策を共有します。
  本パッケージ自身はリトライも `http.Client` も持ちません。

---

## 📦 インストール

```bash
go get github.com/shouni/go-notify
```

---

## 🔧 使い方

### 基本

```go
notifier, err := slack.NewNotifier(httpClient, os.Getenv("SLACK_WEBHOOK_URL"))
if err != nil {
    return err
}

body := notify.NewBody().
    Code("Command", task.Command).
    Code("Job ID", task.JobID).
    Link("History Detail", detailURL, task.JobID).
    Field("Title", task.Title)          // Title が空なら行ごと出ません

err = notifier.Notify(ctx, notify.Message{
    Title: "✅ 生成が完了しました",
    Body:  body.String(),
})
```

### パイプライン通知

成功・失敗・スキップで見出しだけが変わる、という定型を `notify.Pipeline` が担います。

```go
pipeline := notify.NewPipeline(notifier, notify.Titles{
    Success: "✅ 生成が完了しました",
    Failure: "❌ 生成に失敗しました",
    Skipped: "⏭️ 差分がないためスキップしました",
})

// 本文の組み立てが重い場合は事前に打ち切れます
if !pipeline.Enabled() {
    return nil
}

body := notify.NewBody().Code("Job ID", jobID)

pipeline.Success(ctx, body)                 // 見出しのみ差し替え
pipeline.Failure(ctx, body, err)            // 本文末尾に「エラー内容」を追記
pipeline.Skipped(ctx, body, reason)         // 本文末尾に「理由」を追記
```

見出しを実行時の条件で切り替えたい場合（コマンド種別ごとに文言を変える等）は、
`Pipeline` を使わず `Notifier` と `Body` を直接組み合わせてください。

### Body の出力形式

| メソッド | 出力（Markdown） | Slack 表示 |
| :--- | :--- | :--- |
| `Field("Title", "夏の終わり")` | `**Title:** 夏の終わり` | **Title:** 夏の終わり |
| `Code("Command", "compose")` | ``**Command:** `compose` `` | **Command:** `compose` |
| `Link("Detail", url, "job-1")` | `**Detail:** [job-1](url)` | **Detail:** [job-1](url) |
| `Text("素の行")` | `素の行` | 素の行 |
| `Error("エラー内容", err)` | `**エラー内容:**` + 改行 + 内容 | 〃 |
| `Block("エラー詳細", s)` | `**エラー詳細:**` + コードブロック | 〃 |

値が空の場合は行ごと出力されません。`Error` / `Block` は値が無ければ `N/A` を表示し、
本文が既にある場合は 1 行空けてから追記します。
1 行も書き込まれなかった `Body` の `String()` は `N/A` を返します。

### 表示のカスタマイズ

投稿時のユーザー名・アイコン・チャンネルは関数オプションで上書きできます。
設定値をどこから読むか（環境変数など）は呼び出し側の責務です。

```go
notifier, err := slack.NewNotifier(httpClient, webhookURL,
    slack.WithUsername("AP MV"),
    slack.WithIconEmoji(":clapper:"),
    slack.WithChannel("#notifications"),   // Webhook 側の設定を上書き
)
```

---

## 📐 プロジェクト構成

```text
go-notify/
├── notify.go     # Notifier / Message / Disabled: チャネル非依存の抽象
├── body.go       # Body: 通知本文のビルダー（標準 Markdown を出力）
├── pipeline.go   # Pipeline: 成功・失敗・スキップの定型通知
└── slack/        # Slack Incoming Webhook 実装（Block Kit / mrkdwn 変換）
```

新しいチャネルを追加する場合は、`notify.Notifier` を実装したサブパッケージを
追加するだけです。`notify` 側の変更は不要です。

### Slack の記法変換について

`slack` パッケージは、`Body` が出力する標準 Markdown を Slack mrkdwn に変換します。

* `**太字**` → `*太字*`、`## 見出し` → `*見出し*`、`- 項目` → `• 項目`
* `[表示テキスト](URL)` → `<URL|表示テキスト>`
* プレーンテキスト中の `&` `<` `>` を実体参照へエスケープ
  （エラー文中の `<nil>` がリンク構文と誤認されるのを防ぎます）

エスケープ対象は**プレーンテキストのみ**です。`<URL|表示テキスト>` の内側や
`<@U123>` などのメンションは Slack が構文として解釈済みのため変換しません。
GCS の署名付き URL に含まれる `&` をエスケープすると署名が変わって 403 になるため、
この境界は意図的なものです。

---

### 📜 ライセンス

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
