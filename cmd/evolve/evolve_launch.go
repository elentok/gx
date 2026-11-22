package evolve

import (
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
)

// LaunchRow is a launch slower than events.SlowLaunch, or (Ticket empty) the
// epic's median launch, which every epic with launches gets.
type LaunchRow struct {
	Ticket      string `json:"ticket,omitempty"`
	TotalMS     int64  `json:"total_ms"`
	SlowestStep string `json:"slowest_step,omitempty"`
	// SlowestPart is the longest timed part of the agent or prompt step when
	// that is the slowest step; empty for events from before the split.
	SlowestPart   string `json:"slowest_part,omitempty"`
	SlowestPartMS int64  `json:"slowest_part_ms,omitempty"`
}

type launchStep struct {
	name  string
	d     time.Duration
	parts map[string]time.Duration
}

// launchSteps names the four launch steps in the order they run.
func launchSteps(ev runlog.Event) [4]launchStep {
	return [4]launchStep{
		{"worktree", ev.WorktreeStep, nil}, {"deps", ev.DepsStep, nil},
		{"agent", ev.AgentStep, ev.AgentParts}, {"prompt", ev.PromptStep, ev.PromptParts},
	}
}

// slowestPart is the longest part, ties broken by name so output is stable.
func (s launchStep) slowestPart() (string, time.Duration) {
	var name string
	var best time.Duration
	for n, d := range s.parts {
		if d > best || d == best && n < name {
			name, best = n, d
		}
	}
	return name, best
}

// buildLaunchRows reads the launch-timed events. A launch over the threshold
// gets a row; the median always does.
func buildLaunchRows(evs []runlog.Event) []LaunchRow {
	var rows []LaunchRow
	var totals []int
	for _, ev := range evs {
		if ev.Type != string(events.LaunchTimed) {
			continue
		}
		totals = append(totals, int(ev.Total))
		if ev.Total <= events.SlowLaunch {
			continue
		}
		slowest := launchSteps(ev)[0]
		for _, s := range launchSteps(ev) {
			if s.d > slowest.d {
				slowest = s
			}
		}
		part, pd := slowest.slowestPart()
		rows = append(rows, LaunchRow{
			Ticket: ev.Ticket, TotalMS: ev.Total.Milliseconds(), SlowestStep: slowest.name,
			SlowestPart: part, SlowestPartMS: pd.Milliseconds(),
		})
	}
	if len(totals) > 0 {
		rows = append(rows, LaunchRow{TotalMS: time.Duration(medianInt(totals)).Milliseconds()})
	}
	return rows
}

// flagSlowLaunches adds the slow-launch flag to the ticket rows that have a
// launch row. It never changes severity, so it spends none of the deep-read cap.
func flagSlowLaunches(table *InputsTable) {
	for i := range table.Rows {
		r := &table.Rows[i]
		if slices.ContainsFunc(table.Launches, func(l LaunchRow) bool { return l.Ticket == r.ID }) {
			r.Flags = append(r.Flags, flagSlowLaunch)
		}
	}
}

func printLaunchRows(w io.Writer, table InputsTable) error {
	if len(table.Launches) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\n### Launch rows"); err != nil {
		return err
	}
	for _, l := range table.Launches {
		var err error
		if l.Ticket == "" {
			_, err = fmt.Fprintf(w, "launch: median %ds\n", l.TotalMS/1000)
		} else {
			part := ""
			if l.SlowestPart != "" {
				part = fmt.Sprintf(" (%s %ds)", l.SlowestPart, l.SlowestPartMS/1000)
			}
			_, err = fmt.Fprintf(w, "launch: %s %ds, slowest step %s%s\n", l.Ticket, l.TotalMS/1000, l.SlowestStep, part)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
