package tickets

import (
	"regexp"
	"strings"
)

// The headings a park appends to a parked ticket's body.
const (
	NeedsAnswerHeading = "## Needs Answer"
	NeedsRepairHeading = "## Needs Repair"
)

var markdownMarkerPattern = regexp.MustCompile(`[*_` + "`" + `]+`)

// ParkReason is the Park reason of t as the epic renders it: the first
// non-empty line after its Needs Answer or Needs Repair heading, markdown
// markers stripped, unclipped. An error ticket's reason is what broke it. It is
// "" for any other status, and for a parked ticket whose section already retired.
func ParkReason(e Epic, t Ticket) string {
	switch status := e.RenderedStatus(t); status {
	case StatusNeedsAnswer:
		return parkSectionLine(t.Body, NeedsAnswerHeading)
	case StatusNeedsRepair:
		return parkSectionLine(t.Body, NeedsRepairHeading)
	case StatusError:
		for _, msg := range []string{t.ReadErr, t.GraphErr, t.BlockedByErr} {
			if msg != "" {
				return msg
			}
		}
		return "unrecognized status " + t.Status
	}
	return ""
}

// parkSectionLine finds heading as an exact line in body and returns the first
// non-empty line after it, markers stripped, or "" if either isn't found.
func parkSectionLine(body, heading string) string {
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
