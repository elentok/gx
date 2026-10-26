// Package grillround reads the Grilling rounds an agent prints: it splits a
// message into question segments, and counts the rounds of a conversation.
// The server's live round card and the stats counter share this one splitter.
package grillround

import (
	"regexp"
	"strings"
)

const questionFence = "```gx-question"

// marker opens each question of a gx-design grilling round. It counts only at
// the start of a line: a question may quote it inline, in backticks.
const marker = "❓ **Q"

var markerRE = regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(marker))

// RunEndLine matches the closing line of a gx-design run. It only marks that a
// run ended: the text after "Next:" is the agent's own, and nothing reads a
// phase from it.
var RunEndLine = regexp.MustCompile(`(?m)^.*✅ [^:\n]+:.*Next:.*$`)

// Marker is the text that opens a question, for tests that build round text.
const Marker = marker

// HasMarker reports whether text opens a grilling question.
func HasMarker(text string) bool { return markerRE.MatchString(text) }

// SplitFences removes every gx-question fence from text and returns the raw
// blocks it held, in order. An unclosed fence runs to the end of the text.
func SplitFences(text string) (stripped string, blocks []string) {
	var kept []string
	inBlock := false
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case inBlock && strings.TrimSpace(line) == "```":
			inBlock = false
		case inBlock:
			blocks[len(blocks)-1] += line + "\n"
		case strings.TrimSpace(line) == questionFence:
			inBlock = true
			blocks = append(blocks, "")
		default:
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), blocks
}

// Segments splits an agent message holding a Grilling round into one segment
// per question. A segment runs from one line-start marker to the next, or to a
// `---` line.
func Segments(text string) []string {
	var segs []string
	locs := markerRE.FindAllStringIndex(text, -1)
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		// The marker's trailing Q opens the header the number is read from.
		segs = append(segs, untilRule("Q"+text[loc[1]:end]))
	}
	return segs
}

// defaultsHeading is the heading gx-grilling puts over the questions it
// decided unless the person objects. They are not cards.
const defaultsHeading = "decided unless you object"

// IsDefaultsHeading reports whether line opens the defaults section, however
// the agent dresses the heading (`#`, `**`, a trailing colon).
func IsDefaultsHeading(line string) bool {
	line = strings.TrimLeft(strings.TrimSpace(line), "#* ")
	return strings.HasPrefix(strings.ToLower(line), defaultsHeading)
}

// untilRule cuts seg at its first `---` line or defaults heading.
func untilRule(seg string) string {
	var kept []string
	for line := range strings.SplitSeq(seg, "\n") {
		if strings.TrimSpace(line) == "---" || IsDefaultsHeading(line) {
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// defaultsList is the text under the defaults heading, which Segments cuts
// off, or "" when the message has none after its last marker.
func defaultsList(text string) string {
	locs := markerRE.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return ""
	}
	lines := strings.Split(text[locs[len(locs)-1][0]:], "\n")
	for i, line := range lines {
		if IsDefaultsHeading(line) {
			list, _ := SplitFences(strings.Join(lines[i+1:], "\n"))
			return list
		}
	}
	return ""
}
