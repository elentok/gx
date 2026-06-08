package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) handlePushUpdate(msg tea.Msg) (Model, tea.Cmd) {
	next, cmd, result := m.push.Update(msg)
	m.push = next
	if result.Done {
		m.output.Set("Push output", result.Output)
		var notifyCmd tea.Cmd
		if result.Err != nil {
			notifyCmd = notify.Error("push failed: " + result.Err.Error())
		} else {
			notifyCmd = notify.Success("pushed")
		}
		return m, tea.Batch(cmd, notifyCmd, m.refresh(), statusRepoMutatedCmd())
	}
	return m, cmd
}
