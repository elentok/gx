package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) handleBumpUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.bump.Update(msg)
	m.bump = next
	if !result.Done {
		return m, cmd
	}
	if result.Err != nil {
		m.statusMsg = "bump failed: " + result.Err.Error()
		return m, nil
	}
	if result.NewTag == "" {
		return m, nil
	}
	if err := m.push.OpenWithTag(m.worktreeRoot, result.NewTag); err != nil {
		m.statusMsg = err.Error()
		return m, nil
	}
	return m, notify.Success("tag created")
}
