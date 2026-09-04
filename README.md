# 🔔 Go Notify

[![CI](https://github.com/shouni/go-notify/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-notify/actions/workflows/ci.yml)
[![Status](https://img.shields.io/badge/Status-Active-brightgreen)](#)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://go.dev/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-notify)](https://go.dev/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-notify)](https://github.com/shouni/go-notify/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-notify.svg)](https://pkg.go.dev/github.com/shouni/go-notify)

## 🚀 概要 (About) - 本文を組み立てるコードに、チャネルを意識させない

**Go Notify** は、非同期パイプラインの実行結果を通知するための Go ライブラリです。
CLI は持たず、アプリケーションに組み込んで使います。

通知チャネルに依存しない `notify` パッケージと、その実装である `slack` パッケージに
分かれており、**本文を組み立てるコードはチャネルを意識しません**。

Slack 固有の話（記法の変換・リトライ・制限）は [docs/slack.md](docs/slack.md) にまとめてあります。

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
  `httpkit.Poster` 経由です。タイムアウト・SSRF 対策・リトライ方針は
  渡されたクライアントのものをそのまま使い、本パッケージ自身は
  リトライも `http.Client` も持ちません。

## 🚦 使い方 (Usage)

`go get github.com/shouni/go-notify` で入れます。

### 基本

```go
import (
    "github.com/shouni/go-notify/notify"
    "github.com/shouni/go-notify/slack"
)

// Webhook 投稿は非冪等なので、リトライは切っておくのが安全です（docs/slack.md を参照）
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

`Message.Title` は必須です。空のまま送るとエラーになります
（本文の先頭行から見出しを推測することはしません）。

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

pipeline.Success(ctx, body)                 // 本文はそのまま
pipeline.Failure(ctx, body, err)            // 本文末尾に「エラー内容」を追記
pipeline.Skipped(ctx, body, reason)         // reason が非 nil なら「理由」を追記
```

`Failure` と `Skipped` の追記先はコピーなので、**渡した `Body` は書き換わりません**。
上のように 1 つの `Body` を結果ごとに渡し直しても、スキップ通知に前の
「エラー内容」が残る、といったことは起きません。

見出しを実行時の条件で切り替えたい場合（コマンド種別ごとに文言を変える等）は、
`WithTitles` で見出しだけを差し替えた `Pipeline` を派生させます。
元の `Pipeline` は変更されないので、1 つ保持したまま呼び出しごとに切り替えられます。

```go
pipeline.WithTitles(notify.Titles{Success: titleFor(cmd)}).Success(ctx, body)
```

### Body の書き味

各メソッドが 1 行ずつ Markdown を書き足します（どのメソッドが何を出すかは godoc）。
**値が空の場合は行ごと出力されません。**「この項目があれば書く」という `if` を
項目の数だけ並べずに済むのはこのためです。`Error` / `Block` は値が無ければ `N/A` を表示します。
1 行も書き込まれなかった `Body` の `String()` は `N/A` を返します。
`Heading` / `Error` / `Block` は、本文が既にある場合は 1 行空けてから追記します。

件数が可変の値を並べるときは `Field` を繰り返すより `Bullet`、
項目が多くて意味のまとまりで区切りたいときは `Heading` を使います。
`- ` や `## ` を `Text` に手書きする必要はありません。

`Block` の中身は各チャネルの記法変換の対象外で、`- ` や `**` は書き換わりません
（詳細は [docs/slack.md](docs/slack.md)）。

### リンク先 URL を組み立てる

`Link` は URL が空なら行ごと省きます。その契約に合わせて URL を組み立てるのが
`notify.JoinURL` です。ベースが空・パス要素が空・URL として解釈できない、の
いずれでも空文字列を返すので、呼び出し側にリンク行を出すかどうかの分岐が要りません。

```go
body.Link("History Detail", notify.JoinURL(serviceURL, "/web/history", jobID), jobID)
// serviceURL か jobID が空なら URL は空文字列 → この行ごと出ません
```

パス要素は URL エスケープされます（スラッシュは区切りとして残るので、
`"/web/history"` のようなパスをそのまま渡せます）。

### 値をコードスパンにする

`Code` は「ラベル + 単一の値」しか作れません。単位や絵文字を添えたい、
1 行に複数のコードスパンを並べたい場合は `CodeSpan` を使います。

Markdown 記法を知る場所を `notify` パッケージに留めるための出口なので、
呼び出し側でバックティックを直接書かないでください。

```go
body.Field("Seed", notify.CodeSpan(strconv.Itoa(seed))+" 🎲")
body.Field("ブランチ", notify.CodeSpan(base)+" ← "+notify.CodeSpan(feature))
```

### 結果の種別（Level）

`Message.Level` は結果の種別を運びます。`Pipeline` を使えば自動で設定されるため、
呼び出し側で指定する必要はありません。

どう表現するかは各チャネルの判断です（Slack は attachment の色帯にします）。
見出しに `✅` `❌` を書いて結果を示す必要はなくなりますが、残しても構いません。
`Message` を直接組み立てている場合は `LevelNone` のままなので、表示は変わりません。

## 📦 パッケージ構成 (Package Structure)

| パッケージ | 役割 |
| :--- | :--- |
| [`notify`](https://pkg.go.dev/github.com/shouni/go-notify/notify) | チャネル非依存の抽象。`Notifier` / `Message` / `Body` / `Pipeline` |
| [`slack`](https://pkg.go.dev/github.com/shouni/go-notify/slack) | Slack Incoming Webhook 実装 → [docs/slack.md](docs/slack.md) |

`slack` は `notify` に依存しますが、逆はありません。新しいチャネルを追加する場合は
`notify.Notifier` を実装したサブパッケージを足すだけで、`notify` 側の変更は不要です。

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
