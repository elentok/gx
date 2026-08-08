package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// checkAddConfirmedMsg carries a blocked-confirmation modal's acceptance
// (see handleToggleCheck): ticketPath is the ticket the user tried to check,
// blockerPaths are its still-unresolved blockers (Epic.BlockingTickets),
// which must be added alongside it so the checked set never contains a
// ticket without its blockers.
type checkAddConfirmedMsg struct {
	ticketPath   string
	blockerPaths []string
}

// isChecked reports whether the ticket at path is in the Tickets tab's
// independent checked set (ticket 13's decoupled design) — separate from
// queue membership.
func (m Model) isChecked(path string) bool {
	return m.queueStore.IsTicketChecked(path)
}

func (m *Model) setPathsChecked(paths []string, checked bool) error {
	if err := m.queueStore.SetTicketChecked(paths, checked); err != nil {
		return err
	}
	m.refreshQueueSnapshot()
	return nil
}

func (m *Model) refreshQueueSnapshot() {
	snapshot := m.queueStore.Snapshot()
	m.queueStatus, m.checked, m.checkOrder = snapshot.Status, snapshot.TicketChecked, snapshot.TicketCheckOrder
}

func nextCheckOrdinal(checkOrder map[string]uint64) uint64 {
	var next uint64 = 1
	for _, ordinal := range checkOrder {
		if ordinal >= next {
			next = ordinal + 1
		}
	}
	return next
}

func markUnchecked(checked map[string]bool, checkOrder map[string]uint64, path string) {
	delete(checked, path)
	delete(checkOrder, path)
}

// handleToggleCheck answers "space" on the selected row: toggling an epic
// row checks/unchecks all of its tickets; toggling a ticket row checks/
// unchecks it alone, unless checking it would leave an unresolved blocker
// unchecked, in which case a confirmation modal opens first (see
// openBlockedConfirm).
func (m Model) handleToggleCheck() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, nil
	}
	if r.isEpic() {
		if err := m.toggleEpicChecked(r.epicIdx); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		return m, nil
	}
	return m.toggleTicketChecked(r.epicIdx, r.ticketIdx)
}

// toggleEpicChecked checks every non-done ticket in the epic at epicIdx if
// any is currently unchecked, otherwise unchecks them all — standard
// "select all" checkbox-group behavior, except a StatusDone ticket is never
// added to the checked set (it has nothing left to queue). A zero-ticket or
// all-done epic is a no-op either way.
func (m *Model) toggleEpicChecked(epicIdx int) error {
	epic := m.epics[epicIdx]
	if len(epic.Tickets) == 0 {
		return nil
	}
	allChecked := true
	paths := make([]string, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		if epic.RenderedStatus(t) == tickets.StatusDone {
			continue
		}
		paths = append(paths, t.Path)
		if !m.isChecked(t.Path) {
			allChecked = false
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return m.setPathsChecked(paths, !allChecked)
}

// toggleTicketChecked toggles the ticket at (epicIdx, ticketIdx). Unchecking
// is always immediate. Checking a StatusDone ticket is a no-op — it has
// nothing left to queue. Checking a ticket with unresolved blockers
// (Epic.BlockingTickets) instead opens a confirmation modal rather than
// checking it outright — accepting adds the ticket plus its blockers
// (checkAddConfirmedMsg), canceling leaves the checked set unchanged. A
// blocker already in the checked set needs no confirmation, since adding it
// again is a no-op — only blockers that would actually change the checked
// set gate the prompt.
func (m Model) toggleTicketChecked(epicIdx, ticketIdx int) (tea.Model, tea.Cmd) {
	epic := m.epics[epicIdx]
	t := epic.Tickets[ticketIdx]
	if m.isChecked(t.Path) {
		if err := m.setPathsChecked([]string{t.Path}, false); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		return m, nil
	}
	if epic.RenderedStatus(t) == tickets.StatusDone {
		return m, nil
	}

	var blockers []tickets.Ticket
	for _, b := range epic.BlockingTickets(t) {
		if !m.isChecked(b.Path) {
			blockers = append(blockers, b)
		}
	}
	if len(blockers) == 0 {
		if err := m.setPathsChecked([]string{t.Path}, true); err != nil {
			return m, notify.Error("save queue: " + err.Error())
		}
		return m, nil
	}

	blockerPaths := make([]string, len(blockers))
	names := make([]string, len(blockers))
	for i, b := range blockers {
		blockerPaths[i] = b.Path
		names[i] = fmt.Sprintf("%s %s", b.DisplayNumber(), b.Title)
	}
	prompt := fmt.Sprintf(
		"This ticket is blocked by: %s — to add this ticket you must also add its blockers, continue?",
		strings.Join(names, ", "),
	)
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    prompt,
		AcceptCmd: cmdConfirmCheckAdd(t.Path, blockerPaths),
	})
	return m, nil
}

// cmdConfirmCheckAdd returns the tea.Cmd run when the blocked-confirmation
// modal is accepted (see confirm.Options.AcceptCmd).
func cmdConfirmCheckAdd(ticketPath string, blockerPaths []string) tea.Cmd {
	return func() tea.Msg {
		return checkAddConfirmedMsg{ticketPath: ticketPath, blockerPaths: blockerPaths}
	}
}

// handleCheckAddConfirmed applies checkAddConfirmedMsg: the ticket plus every
// one of its blockers join the checked set.
func (m Model) handleCheckAddConfirmed(msg checkAddConfirmedMsg) (tea.Model, tea.Cmd) {
	paths := make([]string, 0, len(msg.blockerPaths)+1)
	paths = append(paths, msg.ticketPath)
	paths = append(paths, msg.blockerPaths...)
	if err := m.setPathsChecked(paths, true); err != nil {
		return m, notify.Error("save queue: " + err.Error())
	}
	return m, nil
}

// autoCheckForkedChildren compares oldEpics (this Model's epics before a
// reload) against newEpics (the reload's result): for every checked ticket
// whose Children field gained new entries since oldEpics — a mid-flight
// fork, per implement/SKILL.md's convention — the newly-appeared sibling(s)
// join the Tickets tab's independent checked set automatically, no
// confirmation modal (ticket 06), unlike toggleTicketChecked's
// blocked-ticket confirmation. A ticket that isn't itself checked forking is
// a no-op: only a fork of already-checked work needs its continuation
// auto-added.
func autoCheckForkedChildren(oldEpics, newEpics []tickets.Epic, store *QueueStore) error {
	if store == nil {
		return nil
	}
	return applyForkedChildren(oldEpics, newEpics, store.IsTicketChecked, store.SetTicketChecked)
}

// autoQueueForkedChildren mirrors autoCheckForkedChildren for the Queue
// tab's own membership concept (Items) instead of the Tickets tab's
// independent checked set: a ticket already queued whose Children field
// gains new entries has its new sibling(s) queued automatically.
func autoQueueForkedChildren(oldEpics, newEpics []tickets.Epic, store *QueueStore) error {
	if store == nil {
		return nil
	}
	return applyForkedChildren(oldEpics, newEpics, store.IsChecked, store.SetChecked)
}

// applyForkedChildren is the shared traversal behind autoCheckForkedChildren
// and autoQueueForkedChildren: isMember/setMember let each caller apply it
// to its own independent membership set (see QueueStore's decoupled
// checked/queued API).
func applyForkedChildren(oldEpics, newEpics []tickets.Epic, isMember func(string) bool, setMember func([]string, bool) error) error {
	oldByPath := make(map[string]tickets.Ticket)
	for _, epic := range oldEpics {
		for _, t := range epic.Tickets {
			oldByPath[t.Path] = t
		}
	}

	var childPaths []string
	for _, epic := range newEpics {
		for _, nt := range epic.Tickets {
			old, ok := oldByPath[nt.Path]
			if !ok || !isMember(old.Path) {
				continue
			}
			for _, childID := range newForkEntries(old.Children, nt.Children) {
				if child, ok := findTicketByIdentifier(epic, childID); ok {
					childPaths = append(childPaths, child.Path)
				}
			}
		}
	}
	return setMember(childPaths, true)
}

// newForkEntries returns the entries present in newChildren but not
// oldChildren, i.e. the sibling IDs a ticket's Children field gained since
// it was last seen.
func newForkEntries(oldChildren, newChildren []string) []string {
	seen := make(map[string]bool, len(oldChildren))
	for _, id := range oldChildren {
		seen[id] = true
	}
	var added []string
	for _, id := range newChildren {
		if !seen[id] {
			added = append(added, id)
		}
	}
	return added
}

// findTicketByIdentifier looks up a ticket within epic by its Identifier
// (e.g. "06a"), used to resolve a newly-observed fork ID to its ticket.Path.
func findTicketByIdentifier(epic tickets.Epic, identifier string) (tickets.Ticket, bool) {
	for _, t := range epic.Tickets {
		if t.Identifier == identifier {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}

// epicChecked reports whether every non-done ticket in epic is currently
// checked (used to render the epic row's own checkbox glyph) — a StatusDone
// ticket can never be checked, so it's excluded from the check. A
// zero-ticket or all-done epic renders unchecked.
func (m Model) epicChecked(epic tickets.Epic) bool {
	if len(epic.Tickets) == 0 {
		return false
	}
	any := false
	for _, t := range epic.Tickets {
		if epic.RenderedStatus(t) == tickets.StatusDone {
			continue
		}
		any = true
		if !m.isChecked(t.Path) {
			return false
		}
	}
	return any
}
