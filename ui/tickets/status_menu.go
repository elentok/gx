package tickets

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// liveOwnedStatuses is what ralph-loop itself writes while it is actively
// scheduling an epic (claiming a ticket, then marking it done) — see ADR
// 0023 (status ownership by writer). Offering either of these in the status
// menu during a live run would invite a person to assert something only gx
// can know mid-run; the rest of the vocabulary (including "open", the write
// unparking needs) stays a person's to set regardless of whether a loop is
// live.
var liveOwnedStatuses = map[schema.Status]bool{
	schema.StatusClaimed: true,
	schema.StatusDone:    true,
}

// allStatusMenuOrder is every status the change-status keymap can offer, in
// menu display order.
var allStatusMenuOrder = []schema.Status{
	schema.StatusOpen,
	schema.StatusClaimed,
	schema.StatusNeedsAnswer,
	schema.StatusNeedsRepair,
	schema.StatusDraft,
	schema.StatusDone,
}

// statusMenuLabels renders each schema.Status's menu label. Kept separate
// from the enum's on-disk spelling so a future rename only has to touch this
// table.
var statusMenuLabels = map[schema.Status]string{
	schema.StatusOpen:        "open",
	schema.StatusClaimed:     "claimed",
	schema.StatusNeedsAnswer: "needs-answer",
	schema.StatusNeedsRepair: "needs-repair",
	schema.StatusDraft:       "draft",
	schema.StatusDone:        "done",
}

// newStatusMenu builds the status menu for ticket: every status but ticket's
// own current one, minus liveOwnedStatuses when live is true (a loop is
// currently scheduling ticket's epic).
func newStatusMenu(ticket tickets.Ticket, live bool) components.MenuState {
	current := schema.Status(strings.ToLower(strings.TrimSpace(ticket.Status)))
	items := make([]components.MenuItem, 0, len(allStatusMenuOrder))
	for _, status := range allStatusMenuOrder {
		if status == current {
			continue
		}
		if live && liveOwnedStatuses[status] {
			continue
		}
		items = append(items, components.MenuItem{Label: statusMenuLabels[status], Value: string(status)})
	}
	return components.MenuState{Items: items, Cursor: 0}
}

// handleChangeStatusKey applies the "s" keymap: opens a status menu for the
// selected ticket row. An epic row (nothing to re-status) or an
// already-terminal selection is a no-op with a toast rather than opening an
// empty menu.
func (m Model) handleChangeStatusKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return m, notify.Info("select a ticket to change its status")
	}
	epic := m.epicAt(r)
	ticket := epic.Tickets[r.ticketIdx]
	live := ralphLoopRegistry.isRunningEpic(epic.Name)
	menu := newStatusMenu(ticket, live)
	if len(menu.Items) == 0 {
		return m, notify.Info("no status changes available for this ticket right now")
	}
	m.statusMenu = menu
	m.statusMenuOpen = true
	return m, nil
}

// handleStatusMenuKey drives the open status menu: navigation/cancel is
// components.UpdateMenu's generic j/k/enter/esc handling, so accepting an
// item re-resolves the selected row (rather than trusting a path captured at
// open time) the same way openImplementConfirm does — nothing can move the
// sidebar selection while a modal owns key input, so re-deriving it here is
// exactly as safe as capturing it up front, without a second field to keep
// in sync.
func (m Model) handleStatusMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, decided, accepted, handled := components.UpdateMenu(msg, m.statusMenu)
	if !handled {
		return m, nil
	}
	m.statusMenu = next
	if !decided {
		return m, nil
	}
	m.statusMenuOpen = false
	if !accepted {
		return m, nil
	}
	status := schema.Status(m.statusMenu.Items[m.statusMenu.Cursor].Value)
	return m.applyStatusChange(status)
}

// applyStatusChange writes status to the selected row's ticket through
// schema.UpdateTicket, mirroring `gx tickets set`'s own guards (cmd/
// tickets_set.go) for the two transitions that can otherwise corrupt
// scheduling state: refusing status=open on an empty body, and status=done
// while blocked_by is still unresolved. Unlike the CLI's --force, the menu
// has no override for the done guard — a person hand-closing a blocked
// ticket from the TUI is exactly the silent-assertion this ticket exists to
// prevent.
func (m Model) applyStatusChange(status schema.Status) (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return m, nil
	}
	epic := m.epicAt(r)
	ticket := epic.Tickets[r.ticketIdx]

	if status == schema.StatusOpen {
		if err := checkTicketBodyBeforeOpen(ticket.Path); err != nil {
			return m, notify.Error(err.Error())
		}
	}
	if status == schema.StatusDone {
		if unresolved := epic.UnresolvedBlockers(ticket); len(unresolved) > 0 {
			return m, notify.Error(fmt.Sprintf(
				"%s has unresolved blocked_by (%s); refusing to mark done",
				ticket.Path, strings.Join(unresolved, ", "),
			))
		}
	}
	return m, cmdApplyTicketStatus(ticket.Path, status)
}

// checkTicketBodyBeforeOpen mirrors cmd/tickets_set.go's checkBodyBeforeOpen:
// refuses to open a ticket with no body, since "nothing schedulable is
// empty" is enforced at the draft-to-open transition rather than at rest
// (schema.Validate keeps accepting a body-less draft).
func checkTicketBodyBeforeOpen(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(schema.ParseBody(string(raw))) == "" {
		return fmt.Errorf("%s has an empty body; refusing to set status=open on a ticket with no content", path)
	}
	return nil
}

// statusChangedMsg reports that cmdApplyTicketStatus's write finished
// successfully, so updateInner can trigger the same reload cmdRefresh uses.
type statusChangedMsg struct{}

// cmdApplyTicketStatus performs the validated write in a Cmd (not inline in
// applyStatusChange) so a filesystem error surfaces as a toast rather than a
// panic on the Update goroutine.
func cmdApplyTicketStatus(path string, status schema.Status) tea.Cmd {
	return func() tea.Msg {
		if err := schema.UpdateTicket(path, func(t *schema.Ticket) {
			t.Status = status
		}); err != nil {
			return notify.Error(err.Error())()
		}
		return statusChangedMsg{}
	}
}

// handleStatusChanged applies statusChangedMsg: reload from disk so the
// sidebar/preview reflect the write, plus a toast naming the new status.
func (m Model) handleStatusChanged() (tea.Model, tea.Cmd) {
	return m, m.cmdLoad()
}

func (m Model) statusMenuView() string {
	prompt := "Choose a status:"
	if r, ok := m.selectedRow(); ok && !r.isEpic() {
		ticket := m.epicAt(r).Tickets[r.ticketIdx]
		prompt = fmt.Sprintf("Choose a status for %q:", ticket.Title)
	}
	return components.RenderMenuModal(
		"Change Status",
		prompt,
		m.statusMenu,
		"",
		ui.ColorBorder,
		ui.ColorBlue,
		ui.ColorSubtle,
		ui.ColorText,
		48,
	)
}
