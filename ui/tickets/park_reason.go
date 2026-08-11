package tickets

import (
	"regexp"
	"strings"

	"github.com/elentok/gx/tickets"
)

// parkReasonMaxRunes caps the ellipsized subtext parkReason returns, rather
// than threading the panel's exact width through buildQueueLines/
// renderQueueTicketRow — RenderPanel's own ansi.Truncate still applies as a
// backstop on narrow terminals, same as blockedBySuffix's unbounded text
// today.
const parkReasonMaxRunes = 60

// needsAnswerHeading/needsRepairHeading are the literal headings
// MarkNeedsAnswerWithReasonAndStub/MarkNeedsRepairWithReason
// (ralphloop/claim.go) append to a parked ticket's body.
const (
	needsAnswerHeading = "## Needs Answer"
	needsRepairHeading = "## Needs Repair"
)

var markdownMarkerPattern = regexp.MustCompile(`[*_` + "`" + `]+`)

// parkReason returns t's park-reason subtext, ellipsized with ellipsisIcon,
// or "" if t isn't currently parked (gated on epic.RenderedStatus(t), not on
// whether the section is still present — a ticket whose section already
// retired stays gated open with no subtext, never falls back to reading
// section presence to decide the row's status) or the section/its first
// content line is missing.
func parkReason(epic tickets.Epic, t tickets.Ticket, ellipsisIcon string) string {
	status := epic.RenderedStatus(t)
	var heading string
	switch status {
	case tickets.StatusNeedsAnswer:
		heading = needsAnswerHeading
	case tickets.StatusNeedsRepair:
		heading = needsRepairHeading
	default:
		return ""
	}

	line := firstContentLineAfterHeading(t.Body, heading)
	if line == "" {
		return ""
	}
	return ellipsize(stripMarkdownMarkers(line), parkReasonMaxRunes, ellipsisIcon)
}

// firstContentLineAfterHeading finds heading as an exact line in body and
// returns the first non-empty line after it, or "" if either isn't found.
func firstContentLineAfterHeading(body, heading string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line != heading {
			continue
		}
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed != "" {
				return trimmed
			}
		}
		return ""
	}
	return ""
}

// stripMarkdownMarkers removes simple emphasis/code markers (*, _, `) — not
// a full markdown parser, matches the modest "markers stripped" ask.
func stripMarkdownMarkers(line string) string {
	return strings.TrimSpace(markdownMarkerPattern.ReplaceAllString(line, ""))
}

// ellipsize caps text to maxRunes runes, appending ellipsisIcon when
// truncated.
func ellipsize(text string, maxRunes int, ellipsisIcon string) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + ellipsisIcon
}
