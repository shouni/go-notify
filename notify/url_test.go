package notify_test

import (
	"testing"

	"github.com/shouni/go-notify/notify"
)

// TestJoinURL verifies the joined result and the cases that return an empty
// string.
func TestJoinURL(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		elems []string
		want  string
	}{
		{
			name:  "ベースとパスとIDを連結する",
			base:  "https://example.com",
			elems: []string{"/web/history", "job-1"},
			want:  "https://example.com/web/history/job-1",
		},
		{
			name:  "ベースの末尾スラッシュは重複しない",
			base:  "https://example.com/",
			elems: []string{"/history", "job-1"},
			want:  "https://example.com/history/job-1",
		},
		{
			name:  "ベースのパスを引き継ぐ",
			base:  "https://example.com/app",
			elems: []string{"history", "job-1"},
			want:  "https://example.com/app/history/job-1",
		},
		{
			name:  "パス要素なしならベースをそのまま返す",
			base:  "https://example.com/app",
			elems: nil,
			want:  "https://example.com/app",
		},
		{
			name:  "ベースが空なら空",
			base:  "",
			elems: []string{"/history", "job-1"},
			want:  "",
		},
		{
			name:  "パス要素のいずれかが空なら空",
			base:  "https://example.com",
			elems: []string{"/history", ""},
			want:  "",
		},
		{
			name:  "URLとして解釈できないベースなら空",
			base:  "://example.com",
			elems: []string{"/history", "job-1"},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notify.JoinURL(tt.base, tt.elems...); got != tt.want {
				t.Errorf("JoinURL(%q, %q) = %q, want %q", tt.base, tt.elems, got, tt.want)
			}
		})
	}
}

// TestJoinURLEscapesPathElements verifies that path elements are URL-escaped,
// so an ID containing a space does not produce mrkdwn whose link breaks
// there.
//
// Slashes alone are not escaped and stay as path separators (url.JoinPath's
// behaviour). Callers pass paths such as "/web/history" as they are, so
// separators and content are not told apart.
func TestJoinURLEscapesPathElements(t *testing.T) {
	got := notify.JoinURL("https://example.com", "/history", "job 1/2")
	want := "https://example.com/history/job%201/2"
	if got != want {
		t.Errorf("JoinURL() = %q, want %q", got, want)
	}
}

// TestJoinURLEmptyResultSkipsLine verifies that passing a URL that failed to
// build to Body.Link omits the whole line, which is the very reason JoinURL
// returns an empty string.
func TestJoinURLEmptyResultSkipsLine(t *testing.T) {
	body := notify.NewBody().
		Code("Job ID", "job-1").
		Link("History Detail", notify.JoinURL("", "/web/history", "job-1"), "job-1")

	want := "**Job ID:** `job-1`"
	if got := body.String(); got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}
