package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/commitops"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

type rewordDetailsMsg struct {
	hash    string
	subject string
	body    string
	err     error
}

func (m *Model) openAmendConfirm() error {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor < 0 || cursor >= len(rows) {
		return nil
	}
	row := rows[cursor]
	return m.commitOps.OpenAmend(m.worktreeRoot, row.commit.FullHash, row.commit.Subject)
}

func (m Model) cmdFetchRewordDetails() tea.Cmd {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor < 0 || cursor >= len(rows) {
		return nil
	}
	row := rows[cursor]
	if row.kind == rowPseudoStatus {
		return nil
	}
	hash := row.commit.FullHash
	root := m.worktreeRoot
	return func() tea.Msg {
		details, err := git.CommitDetailsForRef(root, hash)
		if err != nil {
			return rewordDetailsMsg{err: err}
		}
		return rewordDetailsMsg{hash: hash, subject: details.Subject, body: details.Body}
	}
}

func (m Model) handleRewordDetails(msg rewordDetailsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("reword: " + msg.err.Error())
	}
	return m, m.commitOps.OpenReword(m.worktreeRoot, msg.hash, msg.subject, msg.body)
}

// handleCommitOpsUpdate forwards msg to the amend/reword flow; once it finishes
// OK, log toasts, reloads onto the commit's subject and announces the mutation.
func (m Model) handleCommitOpsUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.commitOps.Update(m.worktreeRoot, msg)
	m.commitOps = next
	if !result.Done || result.Err != nil {
		return m, cmd
	}
	toast := "amended commit"
	if result.Kind == commitops.KindReword {
		toast = "rewrote commit"
	}
	return m, tea.Batch(notify.Success(toast), m.cmdReloadFocusSubject(result.NewSubject), nav.RepoMutated())
}

func (m Model) cmdReloadFocusSubject(subject string) tea.Cmd {
	root := m.worktreeRoot
	store := m.settings.TicketStore
	startRef := m.startRef
	statusDetail := m.pseudoStatusDetail()
	return func() tea.Msg {
		entries, err := git.LogEntries(root, startRef, maxLogEntries)
		if err != nil {
			return reloadMsg{err: err}
		}
		classes, branchDiverged, trunkErr := fetchBranchHistoryClasses(store, root, startRef)
		rows := make([]row, 0, len(entries)+1)
		rows = append(rows, row{kind: rowPseudoStatus, detail: statusDetail})
		for _, entry := range entries {
			rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
		}
		return reloadMsg{rows: rows, branchDiverged: branchDiverged, trunkErr: trunkErr, focusSubject: subject}
	}
}
