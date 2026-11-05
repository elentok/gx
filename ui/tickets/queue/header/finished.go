package header

import (
	"fmt"

	"github.com/elentok/gx/ui"
)

// FinishedLines renders the snapshot's Recently finished epics as dim lines,
// "name project done/total finished 2h ago". They read h.Now at draw time, so
// finishedTickMsg only has to trigger a re-render.
func (h Header) FinishedLines() []string {
	lines := make([]string, 0, len(h.Finished))
	for _, f := range h.Finished {
		ago := ui.DurationCompact(h.Now().Sub(f.CompletedAt))
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
