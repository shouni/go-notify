# 🔔 Go Notify

[![CI](https://github.com/shouni/go-notifier/actions/workflows/ci.yml/badge.svg)](https://github.com/shouni/go-notifier/actions/workflows/ci.yml)
[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/go-notifier)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/go-notify)](https://github.com/shouni/go-notify/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/shouni/go-notify.svg)](https://pkg.go.dev/github.com/shouni/go-notify)
[![Status](https://img.shields.io/badge/Status-Completed-brightgreen)](#)

## 🚀 概要 (About) - 指数バックオフ・堅牢通信対応のマルチチャネル通知ツールキット

**Go Notify** は、複数のチャネル（Slack）に対して**堅牢**（Robust）にメッセージを投稿・通知するための Go 言語製 CLI アプリケーションです。通信の信頼性を最優先し、ビジネスロジックとインフラストラクチャ層を明確に分離した設計を採用しています。

### **主要な機能と特徴:**

* **堅牢な通信基盤**:
  * **`go-http-kit`** をコアに採用。全ての外部通信において、**指数バックオフ**（Exponential Backoff）によるインテリジェントなリトライ処理を備えた HTTP クライアントを共有します。
* **徹底した関心事の分離**:
  * HTTP 通信、リトライロジック、タイムアウト制御を `httpkit.Client` に完全集約。パッケージ側のロジックは通信の不安定さを意識せず、純粋なビジネスルールに集中できます。
* **高度な表現力 (Slack Block Kit)**:
  * Slack への通知は **Block Kit** 形式をフルサポート。Markdown 形式のテキストを Slack 専用のフォーマットへ自動変換し、視認性の高い通知を実現します。

---


### ⚙️ 環境変数設定 (Environment Variables)

#### Slack 設定
| 変数名 | 役割 | デフォルト値 |
| :--- | :--- | :--- |
| **`SLACK_WEBHOOK_URL`** | **(必須)** 投稿先の Webhook URL。 | (なし) |
| **`SLACK_USERNAME`** | 投稿時の表示ユーザー名。 | `Bot` |
| **`SLACK_ICON_EMOJI`** | 投稿時のアイコン絵文字。 | `:robot_face:` |
| **`SLACK_CHANNEL`** | 投稿先のチャンネル（Webhookの設定を上書きする場合）。 | (なし) |

---


## 📐 プロジェクト構成


```text
go-notify/
├── notify.go        # package notify: 通知の抽象 + パイプライン通知パターン
└── slack/           # package slack: Webhook実装（現 go-notifier/pkg/slack）
```

---

### 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。

---
