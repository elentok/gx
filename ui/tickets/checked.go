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

// isChecked reports whether the ticket at path is in the Tickets tab's
// checked set — separate from queue membership.
func (m Model) isChecked(path string) bool {
	return m.checked[path]
}

// setPathsChecked mutates the in-memory checked set. The server has no
// "checked" concept, so the selection deliberately does not survive a restart.
func (m *Model) setPathsChecked(paths []string, checked bool) {
	if m.checked == nil {
		m.checked = map[string]bool{}
	}
	for _, path := range paths {
		if checked {
			m.checked[path] = true
		} else {
			delete(m.checked, path)
		}
	}
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
		m.toggleEpicChecked(r)
		return m, nil
	}
	return m.toggleTicketChecked(r)
}

// eligibleEpicTickets returns epic's tickets that aren't StatusDone — a done
// ticket has nothing left to queue/check, so every "is this epic fully
// checked/queued" walk in this file reasons about this subset alone.
func eligibleEpicTickets(epic tickets.Epic) []tickets.Ticket {
	var out []tickets.Ticket
	for _, t := range epic.Tickets {
		if !epic.RenderedStatus(t).Terminal() {
			out = append(out, t)
		}
	}
	return out
}

// epicChecked reports whether every one of epic's eligible (non-done) tickets
// is checked (also used to render the epic row's own checkbox glyph). A
// zero-eligible epic (no tickets, or all done) renders unchecked: it has
// nothing to be fully checked about.
func (m Model) epicChecked(epic tickets.Epic) bool {
	eligible := eligibleEpicTickets(epic)
	if len(eligible) == 0 {
		return false
	}
	for _, t := range eligible {
		if !m.isChecked(t.Path) {
			return false
		}
	}
	return true
}

// toggleEpicChecked checks every non-done ticket in r's epic if any is
// currently unchecked, otherwise unchecks them all — standard "select all"
// checkbox-group behavior, except a StatusDone ticket is never added to the
// checked set (it has nothing left to queue). A zero-ticket or all-done epic
// is a no-op either way.
func (m *Model) toggleEpicChecked(r row) {
	epic := m.epicAt(r)
	eligible := eligibleEpicTickets(epic)
	paths := make([]string, len(eligible))
	for i, t := range eligible {
		paths[i] = t.Path
	}
	m.setPathsChecked(paths, !m.epicChecked(epic))
}

// toggleTicketChecked toggles r's ticket. Unchecking is always immediate.
// Checking a StatusDone ticket is a no-op — it has nothing left to queue.
// Checking a ticket with unresolved blockers (Epic.BlockingTickets) instead
// opens a confirmation modal rather than checking it outright — accepting
// adds the ticket plus its blockers (checkAddConfirmedMsg), canceling leaves
// the checked set unchanged. A blocker already in the checked set needs no
// confirmation, since adding it again is a no-op — only blockers that would
// actually change the checked set gate the prompt.
func (m Model) toggleTicketChecked(r row) (tea.Model, tea.Cmd) {
	epic := m.epicAt(r)
	t := epic.Tickets[r.ticketIdx]
	if m.isChecked(t.Path) {
		m.setPathsChecked([]string{t.Path}, false)
		return m, nil
	}
	if epic.RenderedStatus(t).Terminal() {
		return m, nil
	}

	var blockers []tickets.Ticket
	for _, b := range epic.BlockingTickets(t) {
		if !m.isChecked(b.Path) {
			blockers = append(blockers, b)
		}
	}
	if len(blockers) == 0 {
		m.setPathsChecked([]string{t.Path}, true)
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
	m.setPathsChecked(paths, true)
	return m, nil
}

// autoCheckForkedChildren compares m.epics (before a reload) against newEpics
// (the reload's result): every ticket that appeared since whose `parent`
// names a checked ticket — a mid-flight fork, per implement/SKILL.md's
// convention — joins the checked set automatically, no confirmation modal
// (ticket 06), unlike toggleTicketChecked's blocked-ticket confirmation. A
// fork of an unchecked ticket is a no-op: only a fork of already-checked work
// needs its continuation auto-added.
func (m *Model) autoCheckForkedChildren(newEpics []tickets.Epic) {
	m.setPathsChecked(m.forkedChildren(newEpics), true)
}

// epicNewTickets pairs a newly-loaded epic with the tickets in it that
// didn't exist in the previous load, plus that epic's own pre-reload state
// (oldEpic, oldEpicOK — false for a brand-new epic) for callers that need to
// look at sibling status as it stood before the new tickets appeared.
type epicNewTickets struct {
	epic       tickets.Epic
	oldEpic    tickets.Epic
	oldEpicOK  bool
	newTickets []tickets.Ticket
}

// diffNewTickets compares oldEpics against newEpics and returns, for every
// epic that gained at least one ticket since the last load, that epic paired
// with its freshly-appeared tickets.
func diffNewTickets(oldEpics, newEpics []tickets.Epic) []epicNewTickets {
	oldByPath := make(map[string]tickets.Epic, len(oldEpics))
	oldTicketPaths := make(map[string]bool)
	for _, epic := range oldEpics {
		oldByPath[epic.Path] = epic
		for _, t := range epic.Tickets {
			oldTicketPaths[t.Path] = true
		}
	}

	var out []epicNewTickets
	for _, epic := range newEpics {
		oldEpic, ok := oldByPath[epic.Path]
		var fresh []tickets.Ticket
		for _, t := range epic.Tickets {
			if !oldTicketPaths[t.Path] {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		out = append(out, epicNewTickets{epic: epic, oldEpic: oldEpic, oldEpicOK: ok, newTickets: fresh})
	}
	return out
}

// forkedChildren returns the paths of newEpics' freshly-appeared tickets
// whose parent is checked.
//
// The fork is detected from the new ticket's own `parent` rather than from
// any list kept on the parent, so a fork still gets picked up when the tool
// that created it never told the parent about it. Both endpoints are
// required to have moved the right way: the child must be newly appeared and
// its parent must already have been loaded before. That second condition is
// what keeps the first reload of a session — where every ticket looks new —
// from mass-adding every fork in the tracker to a membership set the user
// only ever added the parents to.
func (m Model) forkedChildren(newEpics []tickets.Epic) []string {
	var childPaths []string
	for _, group := range diffNewTickets(m.epics, newEpics) {
		if !group.oldEpicOK {
			continue
		}
		oldTicketPaths := make(map[string]bool, len(group.oldEpic.Tickets))
		for _, t := range group.oldEpic.Tickets {
			oldTicketPaths[t.Path] = true
		}
		parents := group.epic.ForkParents()
		for _, nt := range group.newTickets {
			parent, ok := parents.Of(nt)
			if !ok || !oldTicketPaths[parent.Path] || !m.isChecked(parent.Path) {
				continue
			}
			childPaths = append(childPaths, nt.Path)
		}
	}
	return childPaths
}
