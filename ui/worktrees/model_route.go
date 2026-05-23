package worktrees

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	if len(m.worktrees) == 0 {
		return nav.ViewState{}, false
	}
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.worktrees) {
		return nav.ViewState{}, false
	}
	wt := m.worktrees[cursor]
	return nav.ViewState{
		Tab:          nav.TabWorktrees,
		WorktreeRoot: wt.Path,
	}, true
}
