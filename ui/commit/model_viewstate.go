package commit

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	return nav.ViewState{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.ref,
	}, true
}
