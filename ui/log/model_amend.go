package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/notify"
)

func (m *Model) openAmendConfirm() error {
	cursor := m.list.Selected()
	if cursor < 0 || cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[cursor]
	return m.amendConfirm.Open(m.worktreeRoot, row.commit.FullHash, row.commit.Subject)
}

func (m Model) handleAmendUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.amendConfirm.Update(msg)
	m.amendConfirm = next
	if result.Done {
		return m.handleAmendDone(result.Err)
	}
	return m, cmd
}

func (m Model) handleAmendDone(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		return m, notify.Error("Amend failed: " + err.Error())
	}
	return m, tea.Batch(notify.Success("amended commit"), m.cmdReloadFocusSubject(m.amendConfirm.Subject))
}

func (m Model) cmdReloadFocusSubject(subject string) tea.Cmd {
	root := m.worktreeRoot
	startRef := m.startRef
	return func() tea.Msg {
		entries, err := git.LogEntries(root, startRef, maxLogEntries)
		if err != nil {
			return reloadMsg{err: err}
		}
		classes, branchDiverged := fetchBranchHistoryClasses(root, startRef)
		rows := make([]row, 0, len(entries))
		for _, entry := range entries {
			rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
		}
		return reloadMsg{rows: rows, branchDiverged: branchDiverged, focusSubject: subject}
	}
}
