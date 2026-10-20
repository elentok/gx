package tickets

import (
	"fmt"

	"github.com/elentok/gx/ui"
)

// finishedLines renders the snapshot's Recently finished epics as dim lines,
// "name project done/total finished 2h ago". They read m.now at draw time, so
// finishedTickMsg only has to trigger a re-render.
func (m QueueModel) finishedLines() []string {
	lines := make([]string, 0, len(m.finished))
	for _, f := range m.finished {
		ago := ui.DurationCompact(m.now().Sub(f.CompletedAt))
		if ago != "now" {
			ago += " ago"
		}
		progress := fmt.Sprintf("%d/%d", f.Done, f.Total)
		if f.OneOff {
			progress = "one-off"
		}
		lines = append(lines, ui.StyleDim.Render(
			fmt.Sprintf("  %s %s %s finished %s", f.Name, f.Project, progress, ago)))
	}
	return lines
}
