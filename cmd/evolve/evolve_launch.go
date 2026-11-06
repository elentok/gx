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
// epic's median launch when that is slower.
type LaunchRow struct {
	Ticket      string `json:"ticket,omitempty"`
	TotalMS     int64  `json:"total_ms"`
	SlowestStep string `json:"slowest_step,omitempty"`
}

// launchSteps names the four launch steps in the order they run.
func launchSteps(ev runlog.Event) [4]struct {
	name string
	d    time.Duration
} {
	return [4]struct {
		name string
		d    time.Duration
	}{{"worktree", ev.WorktreeStep}, {"deps", ev.DepsStep}, {"agent", ev.AgentStep}, {"prompt", ev.PromptStep}}
}

// buildLaunchRows reads the launch-timed events. A launch over the threshold
// gets a row; so does the median when it is over.
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
		rows = append(rows, LaunchRow{Ticket: ev.Ticket, TotalMS: ev.Total.Milliseconds(), SlowestStep: slowest.name})
	}
	if med := time.Duration(medianInt(totals)); med > events.SlowLaunch {
		rows = append(rows, LaunchRow{TotalMS: med.Milliseconds()})
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
			_, err = fmt.Fprintf(w, "launch: %s %ds, slowest step %s\n", l.Ticket, l.TotalMS/1000, l.SlowestStep)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
