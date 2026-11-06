package ticketwrite

import (
	"strings"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

const (
	selfReportedNoQuestion = "The agent asked for help but left no question. Open the ticket to read its notes."
	zeroCommitSentence     = "The agent finished with no commits and gave no reason."
	blockedPaneSentence    = "The agent waits on a permission prompt. Approve or deny it on the ticket card."

	// fallbackMax is the longest first line shown whole.
	fallbackMax = 100
)

// markedPause is a sentence for a pause source that tags its reason with a
// stable events.Marker. Matching reads the kind and the marker prefix only,
// never the error text after it.
type markedPause struct {
	kind     events.Kind
	marker   events.Marker
	sentence string
}

// markedPauses lists the marker-keyed sentences, checked before the kind-only
// families.
var markedPauses []markedPause

// PauseChatDetail is the chat sentence for a pause. A self-reported pause with
// an open question shows the agent's own summary.
func PauseChatDetail(ticketPath string, kind events.Kind, reason string) string {
	return pauseSentence(ticketPath, kind, reason, markedPauses)
}

func pauseSentence(ticketPath string, kind events.Kind, reason string, marked []markedPause) string {
	for _, m := range marked {
		if m.kind == kind && m.marker.In(reason) {
			return m.sentence
		}
	}
	switch kind {
	case events.SelfReported:
		if ticketPath != "" {
			if evs, err := tickets.ReadLog(ticketPath); err == nil {
				if q := tickets.OpenQuestion(evs); q != nil && q.Summary != "" {
					return q.Summary
				}
			}
		}
		return selfReportedNoQuestion
	case events.ZeroCommit:
		return zeroCommitSentence
	case events.BlockedPane:
		return blockedPaneSentence
	}
	return firstLineCut(reason)
}

// firstLineCut is the reason's first line, cut to fallbackMax characters with
// an ellipsis when longer.
func firstLineCut(reason string) string {
	line, _, _ := strings.Cut(reason, "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > fallbackMax {
		return string(r[:fallbackMax]) + "…"
	}
	return line
}
