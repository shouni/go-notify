package notify

import (
	"net/url"
	"slices"
)

// JoinURL returns base with the path elements joined onto it. It returns an
// empty string when base or any of elems is empty, or when base cannot be
// parsed as a URL.
//
// It returns an empty string rather than an error because Body.Link and
// LinkOrField promise to omit the line when the URL is empty, so callers
// need no branch deciding whether to build the link line. url.JoinPath
// escapes the path elements, but slashes stay as separators.
//
//	body.Link("History Detail", notify.JoinURL(serviceURL, "/web/history", jobID), jobID)
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
