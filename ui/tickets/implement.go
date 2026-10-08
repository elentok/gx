package tickets

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// handleReplaceQueueKey applies bugs-05/03's "r" ("Replace queue") action: it
// opens a confirmation step naming what's about to happen; accepting it runs
// replaceQueuedSelection via handleReplaceQueueConfirmed and switches to the
// Queue tab.
func (m Model) handleReplaceQueueKey() (tea.Model, tea.Cmd) {
	if m.serverMode() {
		return m.handleServerReplaceKey()
	}
	if len(m.checked) == 0 {
		return m, notify.Info("check at least one ticket to build an execution plan")
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "Replace the queue with the checked selection?",
		AcceptCmd: cmdConfirmReplaceQueue(m.worktreeRoot),
	})
	return m, nil
}

// replaceQueueConfirmedMsg carries "r"'s confirmation acceptance: worktreeRoot
// is captured when the modal opened (mirroring checkAddConfirmedMsg's same
// capture-at-open-time approach in checked.go) since the actual queue
// mutation must run against the live Model, not the value m.confirm.Open
// closed over.
type replaceQueueConfirmedMsg struct {
	worktreeRoot string
}

func cmdConfirmReplaceQueue(worktreeRoot string) tea.Cmd {
	return func() tea.Msg {
		return replaceQueueConfirmedMsg{worktreeRoot: worktreeRoot}
	}
}

// handleReplaceQueueConfirmed applies replaceQueueConfirmedMsg: the queue's
// not-yet-started entries are replaced with the checked selection, then the
// app switches to the Queue tab.
func (m Model) handleReplaceQueueConfirmed(msg replaceQueueConfirmedMsg) (tea.Model, tea.Cmd) {
	if err := m.replaceQueuedSelection(); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	return m, cmdOpenQueueTab(msg.worktreeRoot)
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

// replaceQueuedSelection applies ticket 10's "r" replace logic, per bugs-06/03's
// fix to the domain glossary's "Replace queue" entry: every pending
// (not-yet-started) and done (already-finished) queue entry is dropped and
// replaced by the current checked selection. Running/errored entries are left
// exactly as they are, whether or not they're still checked, since a live
// run's own state isn't something Replace should silently discard. Ticket
// 15's EnqueueAndClearChecked also clears every just-queued path from the
// Tickets tab's independent checked set in the same atomic write, so the
// checkboxes visually reset the moment their tickets are queued.
func (m *Model) replaceQueuedSelection() error {
	snapshot := m.queueStore.Snapshot()
	next := make(map[string]queueItemStatus, len(snapshot.Status))
	order := make(map[string]uint64, len(snapshot.Order))
	for path, status := range snapshot.Status {
		if status == queueStatusPending || status == queueStatusDone {
			continue
		}
		next[path] = status
		order[path] = snapshot.Order[path]
	}
	clearedPaths := make([]string, 0, len(m.checked))
	for path := range m.checked {
		clearedPaths = append(clearedPaths, path)
		if m.isTicketDone(path) {
			continue
		}
		if _, exists := next[path]; exists {
			continue
		}
		next[path] = queueStatusPending
		order[path] = m.checkOrder[path]
	}
	if err := m.queueStore.EnqueueAndClearChecked(next, order, clearedPaths); err != nil {
		return err
	}
	m.refreshQueueSnapshot()
	return nil
}

// isTicketDone reports whether path's ticket is already tickets.StatusDone
// within m.epics — a done ticket has nothing left to implement, so
// replaceQueuedSelection excludes it from the checked selection it enqueues.
func (m *Model) isTicketDone(path string) bool {
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if t.Path == path {
				return epic.RenderedStatus(t).Terminal()
			}
		}
	}
	return false
}

func cmdOpenQueueTab(worktreeRoot string) tea.Cmd {
	return nav.Switch(nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: worktreeRoot})
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
