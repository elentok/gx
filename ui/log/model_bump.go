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
		return m, notify.Error("bump failed: " + result.Err.Error())
	}
	if result.NewTag == "" {
		return m, nil
	}
	if err := m.push.OpenWithTag(m.worktreeRoot, result.NewTag); err != nil {
		return m, notify.Error(err.Error())
	}
	return m, notify.Success("tag created")
}
