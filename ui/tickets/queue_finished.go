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
		lines = append(lines, ui.StyleDim.Render(
			fmt.Sprintf("  %s %s %d/%d finished %s", f.Name, f.Project, f.Done, f.Total, ago)))
	}
	return lines
}
