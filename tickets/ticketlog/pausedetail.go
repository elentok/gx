package ticketlog

import (
	"regexp"
	"strings"

	"github.com/elentok/gx/tickets"
)

var markdownMarkerPattern = regexp.MustCompile(`[*_` + "`" + `]+`)

// PauseDetail is the pause text of t as the epic renders it. A ticket paused
// for an answer shows its open question's summary from the log; an old ticket
// that still carries a Needs Answer section falls back to that section. A
// ticket paused for a repair shows the first non-empty line after its Needs
// Repair heading, markdown markers stripped, unclipped. An error ticket's
// text is what broke it. It is "" for any other status, and for a paused
// ticket with neither an open question nor a section.
func PauseDetail(e tickets.Epic, t tickets.Ticket) string {
	switch status := e.RenderedStatus(t); status {
	case tickets.StatusPausedAnswer:
		if evs, err := tickets.ReadLog(t.Path); err == nil {
			if q := tickets.OpenQuestion(evs); q != nil && q.Summary != "" {
				return strings.TrimSpace(markdownMarkerPattern.ReplaceAllString(q.Summary, ""))
			}
		}
		return headingLine(t.Body, tickets.LegacyAnswerHeading)
	case tickets.StatusPausedRepair:
		return headingLine(t.Body, tickets.LegacyRepairHeading)
	case tickets.StatusError:
		for _, msg := range []string{t.ReadErr, t.GraphErr, t.BlockedByErr} {
			if msg != "" {
				return msg
			}
		}
		return "unrecognized status " + t.Status
	}
	return ""
}

// headingLine finds heading as an exact line in body and returns the first
// non-empty line after it, markers stripped, or "" if either isn't found.
func headingLine(body, heading string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line != heading {
			continue
		}
		for _, next := range lines[i+1:] {
			if trimmed := strings.TrimSpace(next); trimmed != "" {
				return strings.TrimSpace(markdownMarkerPattern.ReplaceAllString(trimmed, ""))
			}
		}
		return ""
	}
	return ""
}
