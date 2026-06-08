package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) handleBumpUpdate(msg tea.Msg) (Model, tea.Cmd) {
	next, cmd, result := m.bump.Update(msg)
	m.bump = next
	if !result.Done {
		return m, cmd
	}
	if result.Err != nil {
		m.showGitError(result.Err)
		return m, nil
	}
	if result.NewTag == "" {
		return m, nil
	}
	if err := m.push.OpenWithTag(m.worktreeRoot, result.NewTag); err != nil {
		m.showGitError(err)
		return m, nil
	}
	return m, notify.Success("tag created")
}
