package slack

import (
	"strings"

	"github.com/rivo/uniseg"
)

// truncateGraphemes truncates s to at most maxLen grapheme clusters (what a
// person perceives as one character) and appends suffix only when it cut.
// A maxLen of 0 or less returns an empty string.
//
// It counts clusters rather than runes so that characters made of several
// runes are not broken. Cutting by rune breaks them like this:
//
//	"がぎぐけこ" (NFD, dakuten separated) cut at 1 becomes "か": the dakuten is lost and it reads as another word
//	"👨‍👩‍👧‍👦" (a ZWJ emoji) is split midway, leaving a dangling joiner
//	"👋🏽" (with a skin-tone modifier) loses its modifier and looks different
//
// Whitespace left just before the cut is dropped before suffix is added, so
// the result does not read with a gap before the marker, as in "long body …".
func truncateGraphemes(s string, maxLen int, suffix string) string {
	if maxLen <= 0 {
		return ""
	}

	// Take grapheme clusters one at a time from the start and measure the
	// byte length of maxLen of them. That is one pass, rather than counting
	// clusters first and cutting again.
	count := 0
	offset := 0
	rest := s
	state := -1
	for rest != "" {
		if count == maxLen {
			// maxLen clusters taken and input remains: truncation is needed.
			break
		}
		var cluster string
		cluster, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		count++
		offset += len(cluster)
	}

	// The scan reached the end (the whole string fits in maxLen), so return
	// the original string unchanged.
	if rest == "" {
		return s
	}

	return strings.TrimSpace(s[:offset]) + suffix
}
