package recovery

import (
	"cmp"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// GxSummaryLabel marks an Iteration summaries entry gx wrote, as opposed to
// the agent's own.
const GxSummaryLabel = "gx"

// SummariesNote lists a ticket's earlier iteration summaries for a relaunched
// session's prompt. A ticket with none gets no note. Verify tickets get their
// Verified entries from VerifyResumeNote.
func SummariesNote(t tickets.Ticket) string {
	raw, err := os.ReadFile(t.Path)
	if err != nil {
		return ""
	}
	summaries := schema.ParseIterationSummaries(schema.ParseBody(string(raw)))
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Earlier iterations of this ticket left these summaries (## Iteration summaries):")
	for _, s := range summaries {
		fmt.Fprintf(&b, "\n- %s (%s): did: %s; went wrong: %s; decisions: %s; left: %s",
			s.SessionID, s.Label, s.Did, s.WentWrong, s.Decisions, s.Left)
	}
	return b.String()
}

// RecordGxSummary appends gx's own entry to the ticket's Iteration summaries:
// what gx did since its previous entry, read from the epic's run log. It never
// fails an iteration, so the caller may ignore the error.
func RecordGxSummary(scratchDir string, addr tickets.Address, ticketPath, sessionID string, now time.Time) error {
	evs, _, err := runlog.ReadEvents(scratchDir, addr.Epic)
	if err != nil {
		return err
	}
	return schema.UpdateTicketWithBody(ticketPath, func(_ *schema.Ticket, body *string) {
		var since time.Time
		for _, s := range schema.ParseIterationSummaries(*body) {
			if s.Label == GxSummaryLabel && s.Time.After(since) {
				since = s.Time
			}
		}
		*body = schema.AppendIterationSummary(*body, schema.IterationSummary{
			SessionID: cmp.Or(sessionID, "unknown"),
			Time:      now,
			Label:     GxSummaryLabel,
			Did:       gxDid(evs, addr.ID, since),
			WentWrong: "none recorded",
			Decisions: "none",
			Left:      "none",
		})
	})
}

// gxDid describes the gx actions logged for ticket id after since.
func gxDid(evs []runlog.Event, id string, since time.Time) string {
	var resends, nudges, relaunches, holders int
	for _, ev := range evs {
		if ev.Ticket != id || !ev.Time.After(since) {
			continue
		}
		switch events.Type(ev.Type) {
		case events.PromptNudged:
			resends++
		case events.Nudged, events.StallNudged:
			nudges++
		case events.HumanRelaunch:
			relaunches++
		case events.IterationStarted:
			if ev.Path == "resumed" {
				relaunches++
			}
		case events.Reclaimed, events.StalePaneClosed:
			holders++
		}
	}
	var parts []string
	for _, p := range []struct {
		n    int
		what string
	}{
		{resends, "prompt resends"}, {nudges, "nudges"}, {relaunches, "relaunches"}, {holders, "holder re-adoptions or closures"},
	} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.what))
		}
	}
	if len(parts) == 0 {
		return "launched the agent and settled the run"
	}
	return strings.Join(parts, ", ")
}
