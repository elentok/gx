package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

func (m QueueModel) handleQueueConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m QueueModel) handleQueueConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

// checkedPaths lists every currently checked ticket path, for the "C" clear
// keymap's confirmation prompt and its accepted clear-all.
func (m QueueModel) checkedPaths() []string {
	paths := make([]string, 0, len(m.checked))
	for path := range m.checked {
		paths = append(paths, path)
	}
	return paths
}

// doneCheckedPaths lists every checked ticket path whose status renders as
// done, for the "c" clear-complete keymap — a ticket counts as done either
// through its file's own Status: frontmatter or the queue's durable
// queueStatusDone (mirroring checkedProgress's same OR, so a just-finished
// run counts before the ticket file's frontmatter catches up).
func (m QueueModel) doneCheckedPaths() []string {
	var paths []string
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if !m.checked[t.Path] {
				continue
			}
			if epic.RenderedStatus(t) == tickets.StatusDone || m.queueStatus[t.Path] == queueStatusDone {
				paths = append(paths, t.Path)
			}
		}
	}
	return paths
}

// queueClearConfirmedMsg carries the "C"/"c" clear keymaps' confirmation
// acceptance: paths is the set captured when the modal opened (mirroring
// checkAddConfirmedMsg's same capture-at-open-time approach in checked.go).
type queueClearConfirmedMsg struct {
	paths []string
}

func cmdConfirmQueueClear(paths []string) tea.Cmd {
	return func() tea.Msg {
		return queueClearConfirmedMsg{paths: paths}
	}
}

// handleQueueClearConfirmed applies queueClearConfirmedMsg: every path is
// unchecked, causing any epic left with no checked tickets to drop out of
// buildQueueEntries' output with no further bookkeeping (see its doc
// comment).
func (m QueueModel) handleQueueClearConfirmed(msg queueClearConfirmedMsg) (tea.Model, tea.Cmd) {
	if err := m.clearCheckedPaths(msg.paths); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	m.clampSelected()
	return m, nil
}

func (m *QueueModel) clearCheckedPaths(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	// Also drop each path from m.candidates: buildQueueEntries only ever grows
	// candidates from m.checked (so a row a user unchecked via the Tickets tab
	// stays visible for re-toggling), which would otherwise leave a cleared
	// ticket's row (and its epic, if it was the last one) rendered forever.
	for _, path := range paths {
		delete(m.candidates, path)
	}
	if m.queueStore != nil {
		if err := m.queueStore.SetChecked(paths, false); err != nil {
			return err
		}
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
		m.queueStatus = snapshot.Status
		return nil
	}
	for _, path := range paths {
		markUnchecked(m.checked, m.checkOrder, path)
	}
	return nil
}
