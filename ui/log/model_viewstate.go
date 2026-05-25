package log

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	vs := nav.ViewState{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.startRef,
	}
	cursor := m.list.Selected()
	if cursor >= 0 && cursor < len(m.rows) && m.rows[cursor].kind == rowCommit {
		vs.FocusSubject = m.rows[cursor].commit.Subject
	}
	return vs, true
}
