package status

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentRoute() (nav.Route, bool) {
	route := nav.Route{
		Tab:          nav.TabStatus,
		WorktreeRoot: m.worktreeRoot,
	}
	if file, ok := m.selectedStatusFile(); ok {
		route.InitialPath = file.Path
	}
	return route, true
}
