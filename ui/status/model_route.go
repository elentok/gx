package status

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	route := nav.ViewState{
		Tab:          nav.TabStatus,
		WorktreeRoot: m.worktreeRoot,
	}
	if file, ok := m.selectedStatusFile(); ok {
		route.InitialPath = file.Path
	}
	return route, true
}
