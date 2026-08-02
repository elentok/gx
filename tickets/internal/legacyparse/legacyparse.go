// Package legacyparse implements the bold-line "Type:"/"Blocked by:"/
// "Status:" ticket format's line parsing, shared by tickets.ParseTicket (the
// package's own public API) and tickets/schema's old-format fallback
// (schema.ParseTicket's parseOldFormat). It exists as its own package,
// independent of both, so schema can depend on it without creating an
// import cycle back through tickets.
package legacyparse

import (
	"regexp"
	"strings"
)

// Fields is the bold-line format's metadata plus the remaining raw markdown
// body (metadata lines stripped out).
type Fields struct {
	Type      string
	BlockedBy []string
	Status    string
	Body      string
}

var metadataLineRe = regexp.MustCompile(`(?i)^(Type|Blocked by|Status):\s*(.*)$`)

var blockedByTokenRe = regexp.MustCompile(`\d+[a-zA-Z]*`)

// blockedByParentheticalRe strips a trailing parenthetical annotation from a
// Blocked by: value, e.g. "06 (`tickets.LoopStatus()` must exist — it does,
// as of 06's commit)" -> "06 ", so digit-like tokens mentioned only in the
// prose explanation (e.g. "06's commit") aren't mistaken for a second
// blocker. Uses a greedy `.*` (rather than `[^)]*` like statusParentheticalRe)
// so it strips from the first "(" through the line's final ")" even when the
// annotation contains nested parens, e.g. "LoopStatus()".
var blockedByParentheticalRe = regexp.MustCompile(`\s*\(.*\)\s*$`)

// statusParentheticalRe strips a trailing parenthetical annotation from a
// Status: value, e.g. "resolved (duplicate of #12)" -> "resolved", so the
// status word alone still matches doneStatuses/openStatuses/etc.
var statusParentheticalRe = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// statusEmDashAnnotationRe strips a trailing em-dash annotation from a
// Status: value, e.g. "superseded — split into 04a...md and 04b...md" ->
// "superseded" (per to-tickets' mid-flight-split convention). An em dash
// rather than a hyphen, so this can't mis-strip a hyphenated status word
// like "ready-for-agent". The annotation can run past the Status: line
// itself (metadataLineRe only captures the first line), so this only strips
// what's actually on that line — the rest falls through to the ticket body
// untouched, which is fine since it's prose either way.
var statusEmDashAnnotationRe = regexp.MustCompile(`\s*—.*$`)

// Parse parses a ticket file's raw text into metadata (Type:, Blocked by:,
// Status:) plus the remaining raw markdown body. Metadata lines aren't
// required to be contiguous or lead the file — e.g. wayfinder-style ticket
// templates interleave a `**Status:**` line among prose paragraphs rather
// than stacking it at the very top — so every line is checked, with
// `**bold**` markers stripped before matching so `**Status:** done` and
// `Status: done` parse the same way. A missing Status: line is the valid
// open/unclaimed default, not an error.
func Parse(raw string) Fields {
	var f Fields

	lines := strings.Split(raw, "\n")
	bodyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		m := metadataLineRe.FindStringSubmatch(strings.ReplaceAll(line, "**", ""))
		if m == nil {
			bodyLines = append(bodyLines, line)
			continue
		}
		key := strings.ToLower(m[1])
		value := strings.TrimSpace(m[2])
		switch key {
		case "type":
			f.Type = value
		case "blocked by":
			f.BlockedBy = parseBlockedBy(value)
		case "status":
			value = statusEmDashAnnotationRe.ReplaceAllString(value, "")
			f.Status = strings.TrimSpace(statusParentheticalRe.ReplaceAllString(value, ""))
		}
	}

	f.Body = strings.Join(bodyLines, "\n")
	return f
}

// parseBlockedBy extracts ticket tokens from a "Blocked by:" value, e.g.
// "02, 05" -> ["02", "05"], "04a" -> ["04a"]. A value with no digits (e.g.
// "-" or "None") yields nil. Trailing prose annotations (a parenthetical
// explanation or an em-dash aside, mirroring the Status: conventions above)
// are stripped first so digit-like tokens mentioned only in the explanation
// — e.g. "06's commit" or a markdown link's filename — aren't mistaken for
// additional blockers.
func parseBlockedBy(value string) []string {
	value = blockedByParentheticalRe.ReplaceAllString(value, "")
	value = statusEmDashAnnotationRe.ReplaceAllString(value, "")
	matches := blockedByTokenRe.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	return matches
}
