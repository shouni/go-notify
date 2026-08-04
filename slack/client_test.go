package slack

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/slack-go/slack"
)

// --- Mock 定義 ---

type mockRequester struct {
	postJSONFunc func(ctx context.Context, url string, data any) ([]byte, error)
}

// PostJSONAndFetchBytes はテストで主に使用するメソッドです。
func (m *mockRequester) PostJSONAndFetchBytes(ctx context.Context, url string, data any) ([]byte, error) {
	if m.postJSONFunc == nil {
		return nil, nil
	}
	return m.postJSONFunc(ctx, url, data)
}

// Requester インターフェースを満たすための他のメソッド定義。
func (m *mockRequester) DoRequest(_ *http.Request) ([]byte, error) { return nil, nil }
func (m *mockRequester) FetchBytes(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockRequester) FetchAndDecodeJSON(_ context.Context, _ string, _ any) error {
	return nil
}
func (m *mockRequester) PostRawBodyAndFetchBytes(_ context.Context, _ string, _ []byte, _ string) ([]byte, error) {
	return nil, nil
}

// --- テスト本体 ---

// TestClient_SendTextWithHeader は、ヘッダーとメッセージを明示的に指定した場合の送信テストです。
func TestClient_SendTextWithHeader(t *testing.T) {
	ctx := context.Background()
	webhookURL := "https://hooks.slack.com/services/test"

	tests := []struct {
		name      string
		header    string
		message   string
		setupMock func(m *mockRequester)
		wantErr   bool
	}{
		{
			name:    "正常系: 正しいパラメータで Webhook が呼ばれる",
			header:  "Test Header",
			message: "Test Message",
			setupMock: func(m *mockRequester) {
				m.postJSONFunc = func(_ context.Context, url string, data any) ([]byte, error) {
					// URLの検証
					if url != webhookURL {
						return nil, errors.New("invalid url")
					}
					// 送信データの型と中身の検証
					msg, ok := data.(slack.WebhookMessage)
					if !ok {
						return nil, errors.New("data is not WebhookMessage")
					}
					// フォールバック用テキスト（Text）と、Block Kitのヘッダーが正しいか
					if msg.Text != "Test Header" {
						return nil, errors.New("unexpected fallback text")
					}
					return []byte("ok"), nil
				}
			},
			wantErr: false,
		},
		{
			name:    "異常系: HTTPリクエストに失敗した場合エラーを返す",
			header:  "Error Test",
			message: "Panic",
			setupMock: func(m *mockRequester) {
				m.postJSONFunc = func(_ context.Context, _ string, _ any) ([]byte, error) {
					return nil, errors.New("network error")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mReq := &mockRequester{}
			tt.setupMock(mReq)

			// NewClient のエラーチェック
			client, err := NewClient(mReq, webhookURL)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			err = client.SendTextWithHeader(ctx, tt.header, tt.message)

			if (err != nil) != tt.wantErr {
				t.Errorf("SendTextWithHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestClient_SendText_HeaderGeneration は、本文から自動生成されるヘッダーの加工ロジックをテストします。
func TestClient_SendText_HeaderGeneration(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		message        string
		expectedHeader string
	}{
		{
			name:           "1行目がヘッダーとして採用される",
			message:        "第一行目\n第二行目",
			expectedHeader: "📢 第一行目",
		},
		{
			name: "長い行は切り捨てられる",
			// 50文字を超えるメッセージ
			message: "これは非常に長いタイトルなので50文字くらいでカットされるはずです。カットされてるかな？どこまで続くかな？",
			// 「📢 」を除いた本文部分が、省略記号込みで50文字以内に収まります。
			expectedHeader: "📢 これは非常に長いタイトルなので50文字くらいでカットされるはずです。カットされてるかな？どこま...",
		},
		{
			name:           "空メッセージの場合はデフォルト",
			message:        "",
			expectedHeader: "📢 通知メッセージ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mReq := &mockRequester{}
			var capturedHeader string
			mReq.postJSONFunc = func(_ context.Context, _ string, data any) ([]byte, error) {
				msg, ok := data.(slack.WebhookMessage)
				if ok {
					capturedHeader = msg.Text
				}
				return []byte("ok"), nil
			}

			client, err := NewClient(mReq, "http://dummy")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			err = client.SendText(ctx, tt.message)
			if err != nil {
				t.Fatalf("failed to send text: %v", err)
			}

			if capturedHeader != tt.expectedHeader {
				t.Errorf("Header generation failed.\ngot  = %v\nwant = %v", capturedHeader, tt.expectedHeader)
			}
		})
	}
}

func TestGenerateHeader(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "先頭行の前後空白を除去する",
			message: "  タイトル  \n本文",
			want:    "📢 タイトル",
		},
		{
			name:    "先頭行が空白のみならデフォルト",
			message: "   \n本文",
			want:    "📢 通知メッセージ",
		},
		{
			name:    "空メッセージならデフォルト",
			message: "",
			want:    "📢 通知メッセージ",
		},
		{
			name:    "先頭行が maxHeaderLength を超える場合は切り詰められる",
			message: "これは非常に長いタイトルなので50文字くらいでカットされるはずです。カットされてるかな？どこまで続くかな？\n本文",
			want:    "📢 これは非常に長いタイトルなので50文字くらいでカットされるはずです。カットされてるかな？どこま...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateHeader(tt.message); got != tt.want {
				t.Errorf("generateHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSectionText(t *testing.T) {
	got := buildSectionText("## 見出し\n**重要**\n- item")
	want := "*見出し*\n*重要*\n• item"

	if got != want {
		t.Errorf("buildSectionText() = %q, want %q", got, want)
	}
}
