package log

import (
	"github.com/atotto/clipboard"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

var logClipboardWrite = clipboard.WriteAll

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
	commit, ok := m.selectedCommit()
	if !ok {
		return notify.Warning("no commit selected")
	}
	if err := logClipboardWrite(commit.FullHash); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked commit hash")
}

func (m Model) yankCommitSubject() tea.Cmd {
	commit, ok := m.selectedCommit()
	if !ok {
		return notify.Warning("no commit selected")
	}
	if err := logClipboardWrite(commit.Subject); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked commit subject")
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
		commitMsg := details.Subject
		if details.Body != "" {
			commitMsg = commitMsg + "\n\n" + details.Body
		}
		if err := logClipboardWrite(commitMsg); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())()
		}
		return notify.Info("yanked commit message")()
	}
}
