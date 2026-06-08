package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) handlePullUpdate(msg tea.Msg) (Model, tea.Cmd) {
	next, cmd, result := m.pull.Update(msg)
	m.pull = next
	if result.Done {
		m.output.Set("Pull output", result.Output)
		var notifyCmd tea.Cmd
		if result.Err != nil {
			notifyCmd = notify.Error("pull failed: " + result.Err.Error())
		} else {
			notifyCmd = notify.Success("pulled")
		}
		return m, tea.Batch(notifyCmd, m.refresh(), statusRepoMutatedCmd())
	}
	return m, cmd
}
