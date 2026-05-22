package commit

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentRoute() (nav.Route, bool) {
	return nav.Route{
		Tab:          nav.TabCommit,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.ref,
	}, true
}
