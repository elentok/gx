package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) handlePushUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.push.Update(msg)
	m.push = next
	if result.Done {
		m.output.Set("Push output", result.Output)
		var notifyCmd tea.Cmd
		if result.Err != nil {
			notifyCmd = notify.Error("push failed: " + result.Err.Error())
			return m, tea.Batch(cmd, m.cmdReload(), notifyCmd)
		}
		notifyCmd = notify.Success("pushed")
		return m, tea.Batch(cmd, m.cmdReload(), notifyCmd, nav.RepoMutated())
	}
	return m, cmd
}

func (m Model) handlePullUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.pull.Update(msg)
	m.pull = next
	if result.Done {
		m.output.Set("Pull output", result.Output)
		var notifyCmd tea.Cmd
		if result.Err != nil {
			notifyCmd = notify.Error("pull failed: " + result.Err.Error())
			return m, tea.Batch(cmd, m.cmdReload(), notifyCmd)
		}
		notifyCmd = notify.Success("pulled")
		return m, tea.Batch(cmd, m.cmdReload(), notifyCmd, nav.RepoMutated())
	}
	return m, cmd
}
