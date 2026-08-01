// Package tickets parses and loads the local markdown issue tracker under
// `.scratch/` (see .ai's issue-tracker-local skill for the on-disk
// conventions), mirroring the role the git package plays for ui/prs.
package tickets

import (
	"regexp"
	"strconv"
	"strings"
)

// Ticket is one parsed `<epic>/issues/NN-<slug>.md` file. Number, Title, and
// Path are filled in by the loader from the filename/path, not by ParseTicket
// (which only ever sees the file's raw text).
type Ticket struct {
	// Number is the numeric portion of the ticket identifier, retained for
	// scheduler ordering and legacy callers. Identifier is the full filename
	// prefix, so tickets such as 10a and 10b remain distinct in the UI.
	Number     int
	Identifier string
	Title      string
	Path       string

	Type string
	// BlockedBy holds each "Blocked by:" token as written, e.g. "02" or
	// "04a" (see parseBlockedBy). A bare-number token means the whole
	// number family (Epic.UnresolvedBlockers); a lettered token names one
	// specific sibling.
	BlockedBy []string
	Status    string // raw Status: value; "" means missing (valid open/unclaimed default)
	Body      string // raw markdown after the leading metadata lines, unmodified

	// ReadErr is set when the loader found the file (its name matched
	// "NN-<slug>.md") but couldn't read its contents (I/O error). Non-empty
	// means Type/BlockedBy/Status/Body are all zero-valued - there was no
	// content to parse. Distinct from an unrecognized Status: value, which is
	// a successfully-read file.
	ReadErr string
}

// DisplayNumber returns the filename's complete ticket identifier. Tickets
// constructed outside the loader predate Identifier, so fall back to Number
// for those callers.
func (t Ticket) DisplayNumber() string {
	if t.Identifier != "" {
		return t.Identifier
	}
	return strconv.Itoa(t.Number)
}

var doneStatuses = map[string]bool{
	"done":        true,
	"resolved":    true,
	"wontfix":     true,
	"closed":      true,
	"superseded":  true,
	"implemented": true,
}

// IsDone reports whether the ticket's raw Status collapses into the "done"
// family. Used for epic open/total counts; the full five-state rendered
// status (open/claimed/blocked/needs-info/done) is a later concern.
func (t Ticket) IsDone() bool {
	return doneStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}

// IsSuperseded reports whether the ticket was closed by a mid-flight split
// (see UnresolvedBlockers) rather than by landing work. It's part of the
// IsDone family for scheduling/blocker-resolution purposes, but callers that
// verify a done ticket's work actually landed (e.g. ralph-loop's startup
// reconciliation) need to tell it apart: a superseded ticket never had
// commits to land in the first place.
func (t Ticket) IsSuperseded() bool {
	return supersededStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}

var metadataLineRe = regexp.MustCompile(`(?i)^(Type|Blocked by|Status):\s*(.*)$`)

var blockedByTokenRe = regexp.MustCompile(`\d+[a-zA-Z]*`)

// statusParentheticalRe strips a trailing parenthetical annotation from a
// Status: value, e.g. "resolved (duplicate of #12)" -> "resolved", so the
// status word alone still matches doneStatuses/openStatuses/etc.
var statusParentheticalRe = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// statusEmDashAnnotationRe strips a trailing em-dash annotation from a
// Status: value, e.g. "superseded — split into 04a...md and 04b...md" ->
// "superseded" (per to-tickets' mid-flight-split convention, see
// UnresolvedBlockers). An em dash rather than a hyphen, so this can't
// mis-strip a hyphenated status word like "ready-for-agent". The annotation
// can run past the Status: line itself (metadataLineRe only captures the
// first line), so this only strips what's actually on that line — the rest
// falls through to the ticket body untouched, which is fine since it's
// prose either way.
var statusEmDashAnnotationRe = regexp.MustCompile(`\s*—.*$`)

// ParseTicket parses a ticket file's raw text into metadata (Type:,
// Blocked by:, Status:) plus the remaining raw markdown body. Metadata lines
// aren't required to be contiguous or lead the file — e.g. wayfinder-style
// ticket templates interleave a `**Status:**` line among prose paragraphs
// rather than stacking it at the very top — so every line is checked, with
// `**bold**` markers stripped before matching so `**Status:** done` and
// `Status: done` parse the same way. A missing Status: line is the valid
// open/unclaimed default, not an error.
func ParseTicket(raw string) (Ticket, error) {
	var t Ticket

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
			t.Type = value
		case "blocked by":
			t.BlockedBy = parseBlockedBy(value)
		case "status":
			value = statusEmDashAnnotationRe.ReplaceAllString(value, "")
			t.Status = strings.TrimSpace(statusParentheticalRe.ReplaceAllString(value, ""))
		}
	}

	t.Body = strings.Join(bodyLines, "\n")
	return t, nil
}

// parseBlockedBy extracts ticket tokens from a "Blocked by:" value, e.g.
// "02, 05" -> ["02", "05"], "04a" -> ["04a"]. A value with no digits (e.g.
// "-" or "None") yields nil.
func parseBlockedBy(value string) []string {
	matches := blockedByTokenRe.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	return matches
}
