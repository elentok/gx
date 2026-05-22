package log

import "github.com/elentok/gx/ui/nav"

func (m Model) currentRouteIdentity() (nav.Route, bool) {
	route := nav.Route{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.startRef,
	}
	cursor := m.list.Selected()
	if cursor >= 0 && cursor < len(m.rows) && m.rows[cursor].kind == rowCommit {
		route.FocusSubject = m.rows[cursor].commit.Subject
	}
	return route, true
}
