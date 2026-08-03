package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
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

// isChecked reports whether the ticket at path is in the checked set.
func (m Model) isChecked(path string) bool {
	return m.checked[path]
}

// WithCheckedSet lets the shell share selection across cached tab models.
func (m Model) WithCheckedSet(checked map[string]bool) Model {
	if checked == nil {
		checked = map[string]bool{}
	}
	m.checked = checked
	return m
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
		m.toggleEpicChecked(r.epicIdx)
		return m, nil
	}
	return m.toggleTicketChecked(r.epicIdx, r.ticketIdx)
}

// toggleEpicChecked checks every ticket in the epic at epicIdx if any is
// currently unchecked, otherwise unchecks them all — standard "select all"
// checkbox-group behavior. A zero-ticket epic is a no-op either way.
func (m *Model) toggleEpicChecked(epicIdx int) {
	epic := m.epics[epicIdx]
	if len(epic.Tickets) == 0 {
		return
	}
	if m.checked == nil {
		m.checked = map[string]bool{}
	}
	if m.queueStatus == nil {
		m.queueStatus = map[string]queueItemStatus{}
	}
	allChecked := true
	for _, t := range epic.Tickets {
		if !m.checked[t.Path] {
			allChecked = false
			break
		}
	}
	for _, t := range epic.Tickets {
		if allChecked {
			delete(m.checked, t.Path)
			delete(m.queueStatus, t.Path)
		} else {
			m.checked[t.Path] = true
			if _, ok := m.queueStatus[t.Path]; !ok {
				m.queueStatus[t.Path] = queueStatusPending
			}
		}
	}
	m.persistQueueState()
}

// toggleTicketChecked toggles the ticket at (epicIdx, ticketIdx). Unchecking
// is always immediate. Checking a ticket with unresolved blockers
// (Epic.BlockingTickets) instead opens a confirmation modal rather than
// checking it outright — accepting adds the ticket plus its blockers
// (checkAddConfirmedMsg), canceling leaves the checked set unchanged.
func (m Model) toggleTicketChecked(epicIdx, ticketIdx int) (tea.Model, tea.Cmd) {
	epic := m.epics[epicIdx]
	t := epic.Tickets[ticketIdx]
	if m.checked[t.Path] {
		delete(m.checked, t.Path)
		delete(m.queueStatus, t.Path)
		m.persistQueueState()
		return m, nil
	}

	blockers := epic.BlockingTickets(t)
	if len(blockers) == 0 {
		if m.checked == nil {
			m.checked = map[string]bool{}
		}
		if m.queueStatus == nil {
			m.queueStatus = map[string]queueItemStatus{}
		}
		m.checked[t.Path] = true
		m.queueStatus[t.Path] = queueStatusPending
		m.persistQueueState()
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
	if m.checked == nil {
		m.checked = map[string]bool{}
	}
	if m.queueStatus == nil {
		m.queueStatus = map[string]queueItemStatus{}
	}
	m.checked[msg.ticketPath] = true
	m.queueStatus[msg.ticketPath] = queueStatusPending
	for _, path := range msg.blockerPaths {
		m.checked[path] = true
		m.queueStatus[path] = queueStatusPending
	}
	m.persistQueueState()
	return m, nil
}

// autoCheckSplitChildren compares oldEpics (this Model's epics before a
// reload) against newEpics (the reload's result): for every checked ticket
// whose Split field gained new entries since oldEpics — a mid-flight split,
// per implement/SKILL.md's convention — the newly-appeared sibling(s) join
// checked automatically, no confirmation modal (ticket 06), unlike
// toggleTicketChecked's blocked-ticket confirmation. A ticket that isn't
// itself checked splitting is a no-op: only a split of already-checked work
// needs its continuation auto-added.
func autoCheckSplitChildren(oldEpics, newEpics []tickets.Epic, checked map[string]bool, queueStatus map[string]queueItemStatus) {
	oldByPath := make(map[string]tickets.Ticket)
	for _, epic := range oldEpics {
		for _, t := range epic.Tickets {
			oldByPath[t.Path] = t
		}
	}

	for _, epic := range newEpics {
		for _, nt := range epic.Tickets {
			old, ok := oldByPath[nt.Path]
			if !ok || !checked[old.Path] {
				continue
			}
			for _, childID := range newSplitEntries(old.Split, nt.Split) {
				if child, ok := findTicketByIdentifier(epic, childID); ok {
					checked[child.Path] = true
					if queueStatus != nil {
						if _, ok := queueStatus[child.Path]; !ok {
							queueStatus[child.Path] = queueStatusPending
						}
					}
				}
			}
		}
	}
}

// newSplitEntries returns the entries present in newSplit but not oldSplit,
// i.e. the sibling IDs a ticket's Split field gained since it was last seen.
func newSplitEntries(oldSplit, newSplit []string) []string {
	seen := make(map[string]bool, len(oldSplit))
	for _, id := range oldSplit {
		seen[id] = true
	}
	var added []string
	for _, id := range newSplit {
		if !seen[id] {
			added = append(added, id)
		}
	}
	return added
}

// findTicketByIdentifier looks up a ticket within epic by its Identifier
// (e.g. "06a"), used to resolve a newly-observed split ID to its ticket.Path.
func findTicketByIdentifier(epic tickets.Epic, identifier string) (tickets.Ticket, bool) {
	for _, t := range epic.Tickets {
		if t.Identifier == identifier {
			return t, true
		}
	}
	return tickets.Ticket{}, false
}

// epicChecked reports whether every ticket in epic is currently checked
// (used to render the epic row's own checkbox glyph). A zero-ticket epic
// renders unchecked.
func (m Model) epicChecked(epic tickets.Epic) bool {
	if len(epic.Tickets) == 0 {
		return false
	}
	for _, t := range epic.Tickets {
		if !m.checked[t.Path] {
			return false
		}
	}
	return true
}
