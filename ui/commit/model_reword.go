package commit

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/reword"
)

func (m *Model) openRewordEditor() tea.Cmd {
	if m.details.FullHash == "" {
		m.statusMsg = "no commit loaded"
		return nil
	}
	pushed, _ := git.IsCommitPushed(m.worktreeRoot, m.details.FullHash)
	cmd, tmpFile, original, err := reword.CmdOpenEditor(
		m.worktreeRoot, m.details.FullHash,
		m.details.Subject, m.details.Body, pushed,
	)
	if err != nil {
		m.statusMsg = "reword: " + err.Error()
		return nil
	}
	m.rewordTmpFile = tmpFile
	m.rewordOrigMsg = original
	return cmd
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
	m.rewordNewSubject = strings.SplitN(newMsg, "\n", 2)[0]
	cmd, err := m.reword.StartRunning(m.worktreeRoot, m.details.FullHash, m.details.Subject, newMsg)
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
	return m, nav.Replace(nav.Route{
		Kind:         nav.RouteLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          "HEAD",
	})
}
