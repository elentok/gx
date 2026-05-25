package status

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	vs := nav.ViewState{
		Tab:          nav.TabStatus,
		WorktreeRoot: m.worktreeRoot,
	}
	if file, ok := m.selectedStatusFile(); ok {
		vs.InitialPath = file.Path
	}
	return vs, true
}
