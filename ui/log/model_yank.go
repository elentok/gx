package log

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/commitops"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func (m Model) selectedCommit() (git.LogEntry, bool) {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if len(rows) == 0 || cursor < 0 || cursor >= len(rows) {
		return git.LogEntry{}, false
	}
	r := rows[cursor]
	if r.kind != rowCommit {
		return git.LogEntry{}, false
	}
	return r.commit, true
}

func (m Model) yankCommitHash() tea.Cmd {
	commit, _ := m.selectedCommit()
	return commitops.YankHash(commit.FullHash)
}

func (m Model) yankCommitSubject() tea.Cmd {
	commit, _ := m.selectedCommit()
	return commitops.YankSubject(commit.Subject)
}

func (m Model) yankCommitMessage() tea.Cmd {
	commit, ok := m.selectedCommit()
	if !ok {
		return notify.Warning("no commit selected")
	}
	worktreeRoot := m.worktreeRoot
	hash := commit.FullHash
	return func() tea.Msg {
		details, err := git.CommitDetailsForRef(worktreeRoot, hash)
		if err != nil {
			return notify.Error("failed to load commit: " + err.Error())()
		}
		return commitops.YankMessage(details.Subject, details.Body)()
	}
}
