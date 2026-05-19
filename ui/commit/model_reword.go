package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

func (m *Model) openRewordEditor() tea.Cmd {
	if m.details.FullHash == "" {
		return notify.Warning("no commit loaded")
	}
	pushed, _ := git.IsCommitPushed(m.worktreeRoot, m.details.FullHash)
	cmd, err := m.reword.CmdOpenEditor(
		m.worktreeRoot, m.details.FullHash,
		m.details.Subject, m.details.Body, pushed,
	)
	if err != nil {
		return notify.Error("reword: " + err.Error())
	}
	return cmd
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
	return m, nav.Replace(nav.Route{
		Kind:         nav.RouteLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          "HEAD",
		FocusSubject: m.reword.NewSubject,
	})
}
