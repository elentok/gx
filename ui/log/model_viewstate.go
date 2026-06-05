package log

import "github.com/elentok/gx/ui/nav"

func (m Model) CurrentViewState() (nav.ViewState, bool) {
	vs := nav.ViewState{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          m.startRef,
	}
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor >= 0 && cursor < len(rows) && rows[cursor].kind == rowCommit {
		vs.FocusSubject = rows[cursor].commit.Subject
	}
	return vs, true
}
