package tickets

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// handleReplaceQueueKey applies bugs-05/03's "r" ("Replace queue") action.
// The server's queue is the only queue, so without one there is nothing to
// replace.
func (m Model) handleReplaceQueueKey() (tea.Model, tea.Cmd) {
	if m.serverMode() {
		return m.handleServerReplaceKey()
	}
	return m, notify.Info("start the server to queue tickets")
}

// handleAddToQueueKey applies ticket 10's "a" ("Add to queue") action. Only
// the server runs epics, so without one there is no live run to add to.
func (m Model) handleAddToQueueKey() (tea.Model, tea.Cmd) {
	if m.serverMode() {
		return m.handleServerEnqueueKey()
	}
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	return m, notify.Info(fmt.Sprintf("epic %q isn't running", m.epicAt(r).Name))
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

func (m Model) handleImplementSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if len(m.implementingEpics) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.implementSpinner, cmd = m.implementSpinner.Update(msg)
	return m, cmd
}
