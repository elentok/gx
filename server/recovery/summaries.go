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
		c := countGxEvents(evs, addr.ID, since)
		*body = schema.AppendIterationSummary(*body, schema.IterationSummary{
			SessionID: cmp.Or(sessionID, "unknown"),
			Time:      now,
			Label:     GxSummaryLabel,
			Did:       gxDid(c),
			WentWrong: gxWentWrong(c),
			Decisions: "none",
			Left:      "none",
		})
	})
}

// gxCounts is how often gx acted on one ticket since a point in time.
type gxCounts struct {
	resends, nudges, relaunches, holders                          int
	launchFailures, escalations, finishBlocks, refreshes, retries int
}

func countGxEvents(evs []runlog.Event, id string, since time.Time) gxCounts {
	var c gxCounts
	for _, ev := range evs {
		if ev.Ticket != id || !ev.Time.After(since) {
			continue
		}
		switch events.Type(ev.Type) {
		case events.PromptNudged:
			c.resends++
		case events.Nudged, events.StallNudged:
			c.nudges++
		case events.HumanRelaunch:
			c.relaunches++
		case events.IterationStarted:
			if ev.Path == "resumed" {
				c.relaunches++
			}
		case events.Reclaimed, events.StalePaneClosed:
			c.holders++
		case events.LaunchFailed:
			c.launchFailures++
		case events.RecoveryEscalated:
			c.escalations++
		case events.FinishBlocked:
			c.finishBlocks++
		case events.SkillsRefreshed:
			c.refreshes++
		case events.LaunchRetried:
			c.retries++
		}
	}
	return c
}

type countPart struct {
	n    int
	what string
}

func joinCounts(parts []countPart, none string) string {
	var out []string
	for _, p := range parts {
		if p.n > 0 {
			out = append(out, fmt.Sprintf("%d %s", p.n, p.what))
		}
	}
	if len(out) == 0 {
		return none
	}
	return strings.Join(out, ", ")
}

// gxDid describes the gx actions logged for ticket id after since.
func gxDid(c gxCounts) string {
	return joinCounts([]countPart{
		{c.resends, "prompt resends"}, {c.nudges, "nudges"}, {c.relaunches, "relaunches"}, {c.holders, "holder re-adoptions or closures"},
	}, "launched the agent and settled the run")
}

// gxWentWrong lists how often gx had to step in; the singular forms differ
// from the plural only where the count is one.
func gxWentWrong(c gxCounts) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	}
	return joinCounts([]countPart{
		{c.resends, plural(c.resends, "prompt resend", "prompt resends")},
		{c.relaunches, plural(c.relaunches, "relaunch", "relaunches")},
		{c.launchFailures, plural(c.launchFailures, "launch failure", "launch failures")},
		{c.escalations, plural(c.escalations, "recovery escalation", "recovery escalations")},
		{c.finishBlocks, plural(c.finishBlocks, "finish block", "finish blocks")},
		{c.refreshes, plural(c.refreshes, "skill refresh", "skill refreshes")},
		{c.retries, plural(c.retries, "skill retry", "skill retries")},
	}, "none recorded")
}
