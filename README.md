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
import (
    "github.com/shouni/go-notify/notify"
    "github.com/shouni/go-notify/slack"
)

// Webhook 投稿は非冪等なので、リトライは切っておくのが安全です（下記の注意を参照）
notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), os.Getenv("SLACK_WEBHOOK_URL"))
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
`WithTitles` で見出しだけを差し替えた `Pipeline` を派生させます。
元の `Pipeline` は変更されないので、1 つ保持したまま呼び出しごとに切り替えられます。

```go
pipeline.WithTitles(notify.Titles{Success: titleFor(cmd)}).Success(ctx, body)
```

### Body の出力形式

| メソッド | 出力（Markdown） | Slack 表示 |
| :--- | :--- | :--- |
| `Field("Title", "サンプル")` | `**Title:** サンプル` | **Title:** サンプル |
| `Code("Command", "compose")` | ``**Command:** `compose` `` | **Command:** `compose` |
| `Link("Detail", url, "job-1")` | `**Detail:** [job-1](url)` | **Detail:** [job-1](url) |
| `LinkOrField("Output", url, uri)` | url があればリンク、無ければ素の値 | 〃 |
| `Text("素の行")` | `素の行` | 素の行 |
| `Error("エラー内容", err)` | `**エラー内容:**` + 改行 + 内容 | 〃 |
| `Block("実行ログ", s)` | `**実行ログ:**` + フェンス付きコードブロック | 〃 |

値が空の場合は行ごと出力されません。`Error` / `Block` は値が無ければ `N/A` を表示し、
本文が既にある場合は 1 行空けてから追記します。
1 行も書き込まれなかった `Body` の `String()` は `N/A` を返します。

`Block` の中身は各チャネルの記法変換の対象外です。コマンド出力やログを
原文のまま見せるための入口なので、`- ` や `**` が書き換わることはありません。

### 値をコードスパンにする

`Code` は「ラベル + 単一の値」しか作れません。単位や絵文字を添えたい、
1 行に複数の等幅の値を並べたい場合は `CodeSpan` を使います。
本文の Markdown 記法を知る場所を `notify` パッケージの中に留めるための出口なので、
呼び出し側でバックティックを直接書かないでください。

```go
body.Field("Seed", notify.CodeSpan(strconv.Itoa(seed))+" 🎲")
body.Field("ブランチ", notify.CodeSpan(base)+" ← "+notify.CodeSpan(feature))
```

### 結果の種別（Level）

`Message.Level` は結果の種別を運びます。`Pipeline` を使えば自動で設定されるため、
呼び出し側で指定する必要はありません。

| Level | Pipeline のメソッド | Slack の表示 |
| :--- | :--- | :--- |
| `LevelSuccess` | `Success` | attachment の色帯 `good`（緑） |
| `LevelFailure` | `Failure` | `danger`（赤） |
| `LevelSkipped` | `Skipped` | `warning`（黄） |
| `LevelNone`（ゼロ値） | — | 色なし。従来どおりトップレベル blocks |

見出しに `✅` `❌` を書いて結果を示す必要はなくなりますが、残しても構いません。
`Message` を直接組み立てている場合は `LevelNone` のままなので、表示は変わりません。

### 表示のカスタマイズ

投稿時のユーザー名・アイコン・チャンネルは関数オプションで上書きできます。
設定値をどこから読むか（環境変数など）は呼び出し側の責務です。

```go
notifier, err := slack.NewNotifier(httpClient, webhookURL,
    slack.WithUsername("Release Bot"),
    slack.WithIconEmoji(":rocket:"),
    slack.WithChannel("#notifications"),   // Webhook 側の設定を上書き
)
```

**既定値はありません。** 指定しなければこれらは送られず、Webhook を持つ
Slack アプリ自身の名前とアイコンで投稿されます。

> ⚠️ **Slack アプリ経由で作成した Incoming Webhook は、これらの上書きを一切受け付けません。**
> 投稿先チャンネル・表示名・アイコンは常にアプリの設定を継承します
> （[公式ドキュメント](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks)）。
> オプションが効くのは旧来の custom integration 版の Webhook だけです。
>
> サービスごとに表示を変えたい場合は **Slack アプリを分けて**ください。
> 1 つのアプリのまま投稿ごとに表示を変えるには、Webhook ではなく
> `chat.postMessage`（`chat:write.customize` スコープ）が必要です。本パッケージは対応していません。

---

## 📐 プロジェクト構成

```text
go-notify/
├── notify/       # チャネル非依存の抽象
│   ├── notify.go     # Notifier / Message / Level / Disabled
│   ├── body.go       # Body: 通知本文のビルダー（標準 Markdown を出力）
│   └── pipeline.go   # Pipeline: 成功・失敗・スキップの定型通知
└── slack/        # Slack Incoming Webhook 実装（Block Kit / mrkdwn 変換）
```

新しいチャネルを追加する場合は、`notify.Notifier` を実装したサブパッケージを
追加するだけです。`notify` 側の変更は不要です。

### ⚠️ リトライについて

**Webhook への投稿は非冪等です。** 成功するたびに新しいメッセージが作られるため、
Slack には届いたのにレスポンスを取りこぼしてリトライすると、同じ通知が二重に投稿されます。

本ライブラリはリトライ方針を持たず、渡された `httpkit.Requester` をそのまま使います。
`httpkit.New(timeout)` は既定でリトライが有効なので、**他の用途と 1 つのクライアントを
共有していると重複投稿が起こり得ます**。

`WithoutRetry` で派生させれば、既存のクライアントのタイムアウト・SSRF 対策・
コネクションプールを共有したまま、通知経路だけリトライを切れます。

```go
notifier, err := slack.NewNotifier(httpClient.WithoutRetry(), webhookURL)
```

クライアント全体でリトライが不要なら `httpkit.New(timeout, httpkit.WithNoRetry())` でも構いません。

「通知の取りこぼし」と「重複投稿」のどちらを避けたいかはアプリケーション側の判断なので、
ライブラリでは決めていません。

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

記法変換の対象外がもう 1 つあります。フェンス（行頭の ` ``` `）で囲まれた
コードブロックの中身です。`Body.Block` はコマンド出力やログを原文のまま
見せるためのものなので、そこで `- ` が `• ` に、`**text**` が `*text*` に
書き換わると、貼った本人が見たい原文が壊れます。
ただし `&` `<` `>` のエスケープはブロック内でも必要なため、記法変換とは分けて適用します。

---

### 📜 ライセンス

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
