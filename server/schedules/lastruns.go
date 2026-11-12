package schedules

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// LastRunsLimit is how many rows Last runs shows.
const LastRunsLimit = 10

// lateAfter is how long after its set time a run may be created and still count as on time.
const lateAfter = time.Minute

// The status word already says Skipped, so the texts do not repeat it.
var reasonText = map[string]string{
	ReasonProjectNotFound:           "The project is gone.",
	ReasonPreviousRunStarted:        "The previous run was still running.",
	ReasonPreviousRunNeedsAttention: "The previous run needs attention.",
}

func skipText(reason string, lateLimitHours float64) string {
	if reason == ReasonLate {
		return "Past the " + strconv.FormatFloat(lateLimitHours, 'f', -1, 64) + " h Late limit."
	}
	return reasonText[reason]
}

type lastRow struct {
	run  wire.ScheduleRun
	at   time.Time
	path string // the One-off's file, to read its Result
}

// LastRuns is the Schedule's newest ten runs: its One-offs from every machine
// sharing the store (live and archived) plus the skips this machine logged.
func LastRuns(ticketStore string, st *State, k Key, lateLimitHours float64) ([]wire.ScheduleRun, error) {
	live, archived, err := tickets.LoadOneOffs(filepath.Join(ticketStore, k.Project))
	if err != nil {
		return nil, err
	}
	prefix := DedupeKey(k, "")
	var rows []lastRow
	for _, e := range []struct {
		epic string
		tks  []tickets.Ticket
	}{{tickets.OneOffsEpic, live.Tickets}, {tickets.ArchivedOneOffsEpic, archived.Tickets}} {
		for _, t := range e.tks {
			if strings.HasPrefix(t.Unique, prefix) {
				rows = append(rows, oneOffRow(k.Project, e.epic, t))
			}
		}
	}
	for _, sk := range st.Skips(k) {
		rows = append(rows, lastRow{
			at: sk.SetTime,
			run: wire.ScheduleRun{
				ScheduledFor:     sk.SetTime.Format(time.RFC3339),
				CreatedAt:        sk.LoggedAt.Format(time.RFC3339),
				Status:           wire.ScheduleRunSkipped,
				Reason:           sk.Reason,
				ReasonText:       skipText(sk.Reason, lateLimitHours),
				AttentionAddress: sk.Blocking,
			},
		})
	}
	slices.SortStableFunc(rows, func(a, b lastRow) int { return b.at.Compare(a.at) })
	if len(rows) > LastRunsLimit {
		rows = rows[:LastRunsLimit]
	}
	out := make([]wire.ScheduleRun, len(rows))
	for i, r := range rows {
		if r.path != "" {
			r.run.Result = resultLine(r.path)
		}
		out[i] = r.run
	}
	return out, nil
}

func oneOffRow(project, epic string, t tickets.Ticket) lastRow {
	run := wire.ScheduleRun{
		Address: tickets.Address{Project: project, Epic: epic, ID: t.DisplayNumber()}.String(),
		Status:  oneOffStatus(t),
	}
	if !t.ScheduledFor.IsZero() {
		run.ScheduledFor = t.ScheduledFor.Format(time.RFC3339)
	}
	if !t.CreatedAt.IsZero() {
		run.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	at := t.ScheduledFor
	if at.IsZero() {
		at = t.CreatedAt
	}
	return lastRow{run: run, at: at, path: t.FilePath()}
}

func oneOffStatus(t tickets.Ticket) wire.ScheduleRunStatus {
	switch t.Status {
	case "claimed":
		return wire.ScheduleRunRunning
	case "done":
		if !t.ScheduledFor.IsZero() && t.CreatedAt.Sub(t.ScheduledFor) > lateAfter {
			return wire.ScheduleRunRanLate
		}
		return wire.ScheduleRunFinished
	case "paused", "needs-answer", "needs-repair":
		return wire.ScheduleRunNeedsAnswer
	case "cancelled":
		return wire.ScheduleRunCancelled
	default:
		return wire.ScheduleRunQueued
	}
}

// resultLine is the first non-empty line of the ticket's Result section.
func resultLine(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(schema.Section(schema.ParseBody(string(raw)), "Result"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
