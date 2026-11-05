package tickets

import (
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

// checkedPaths lists every currently checked ticket path, for the "C" clear
// keymap's confirmation prompt and its accepted clear-all.
func (m QueueModel) checkedPaths() []string {
	paths := make([]string, 0, len(m.rows.checked))
	for path := range m.rows.checked {
		paths = append(paths, path)
	}
	return paths
}

// doneCheckedPaths lists every checked ticket path whose status renders as
// done, for the "c" clear-complete keymap.
func (m QueueModel) doneCheckedPaths() []string {
	var paths []string
	for _, epic := range m.rows.epics {
		for _, t := range epic.Tickets {
			if m.rows.checked[t.Path] && rowfmt.TicketStatus(epic, t).Terminal() {
				paths = append(paths, t.Path)
			}
		}
	}
	return paths
}
