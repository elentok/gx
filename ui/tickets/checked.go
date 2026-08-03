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
		} else {
			m.checked[t.Path] = true
		}
	}
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
		return m, nil
	}

	blockers := epic.BlockingTickets(t)
	if len(blockers) == 0 {
		if m.checked == nil {
			m.checked = map[string]bool{}
		}
		m.checked[t.Path] = true
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
	m.checked[msg.ticketPath] = true
	for _, path := range msg.blockerPaths {
		m.checked[path] = true
	}
	return m, nil
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
