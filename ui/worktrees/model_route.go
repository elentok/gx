package worktrees

import "github.com/elentok/gx/ui/nav"

func (m Model) currentRouteIdentity() (nav.Route, bool) {
	if len(m.worktrees) == 0 {
		return nav.Route{}, false
	}
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.worktrees) {
		return nav.Route{}, false
	}
	wt := m.worktrees[cursor]
	return nav.Route{
		Tab:          nav.TabWorktrees,
		WorktreeRoot: wt.Path,
	}, true
}
