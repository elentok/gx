package log

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/reword"
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
		m.statusMsg = "reword: " + msg.err.Error()
		return m, nil
	}
	cmd, tmpFile, original, err := reword.CmdOpenEditor(m.worktreeRoot, msg.hash, msg.subject, msg.body, msg.pushed)
	if err != nil {
		m.statusMsg = "reword: " + err.Error()
		return m, nil
	}
	m.rewordTmpFile = tmpFile
	m.rewordOrigMsg = original
	m.rewordHash = msg.hash
	m.rewordSubject = msg.subject
	return m, cmd
}

func (m Model) handleRewordEditorDone(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		m.statusMsg = "reword: editor failed: " + err.Error()
		return m, nil
	}
	changed, newMsg, err := reword.ReadResult(m.rewordTmpFile, m.rewordOrigMsg)
	if err != nil {
		m.statusMsg = "reword: " + err.Error()
		return m, nil
	}
	if !changed {
		m.statusMsg = "reword: no changes"
		return m, nil
	}
	newSubject := strings.SplitN(newMsg, "\n", 2)[0]
	m.rewordNewSubject = newSubject
	cmd, err := m.reword.StartRunning(m.worktreeRoot, m.rewordHash, m.rewordSubject, newMsg)
	if err != nil {
		m.statusMsg = "reword: " + err.Error()
		return m, nil
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
		m.statusMsg = "reword failed: " + err.Error()
		return m, nil
	}
	m.statusMsg = "rewrote commit"
	return m, m.cmdReloadFocusSubject(m.rewordNewSubject)
}
