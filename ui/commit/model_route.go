package commit

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	return nav.ViewState{
		Tab:          nav.TabCommit,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.ref,
	}, true
}
