package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

type rewordDetailsMsg struct {
	hash    string
	subject string
	body    string
	pushed  bool
	err     error
}

func (m Model) cmdFetchRewordDetails() tea.Cmd {
	cursor := m.list.Selected()
	if cursor < 0 || cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[cursor]
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
		pushed, _ := git.IsCommitPushed(root, hash)
		return rewordDetailsMsg{
			hash:    hash,
			subject: details.Subject,
			body:    details.Body,
			pushed:  pushed,
		}
	}
}

func (m Model) handleRewordDetails(msg rewordDetailsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("reword: " + msg.err.Error())
	}
	cmd, err := m.reword.CmdOpenEditor(m.worktreeRoot, msg.hash, msg.subject, msg.body, msg.pushed)
	if err != nil {
		return m, notify.Error("reword: " + err.Error())
	}
	return m, cmd
}

func (m Model) handleRewordEditorDone(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		return m, notify.Error("reword: editor failed: " + err.Error())
	}
	changed, newMsg, err := m.reword.ReadEditorResult()
	if err != nil {
		return m, notify.Error("reword: " + err.Error())
	}
	if !changed {
		return m, notify.Info("reword: no changes")
	}
	cmd, err := m.reword.StartRunning(m.worktreeRoot, newMsg)
	if err != nil {
		return m, notify.Error("reword: " + err.Error())
	}
	return m, cmd
}

func (m Model) handleRewordRunningUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.reword.Update(msg)
	m.reword = next
	if result.Done {
		return m.handleRewordDone(result.Err)
	}
	return m, cmd
}

func (m Model) handleRewordDone(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		return m, notify.Error("reword failed: " + err.Error())
	}
	return m, tea.Batch(notify.Success("rewrote commit"), m.cmdReloadFocusSubject(m.reword.NewSubject), nav.RepoMutated())
}
