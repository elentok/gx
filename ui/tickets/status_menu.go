package tickets

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
)

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
	return m, statusChangeCmd(m.serverAPI, m.settings.TicketStore, epic, epic.Tickets[r.ticketIdx], status)
}

// statusChangeCmd is applyStatusChange's guarded write for any tab's row.
func statusChangeCmd(api ServerAPI, store string, epic tickets.Epic, ticket tickets.Ticket, status schema.Status) tea.Cmd {
	file, address := resolveTicketPath(store, ticket.Path)

	if status == schema.StatusOpen {
		if err := checkTicketBodyBeforeOpen(file); err != nil {
			return notify.Error(err.Error())
		}
	}
	if status == schema.StatusDone {
		// ADR 0030: a map epic's tickets never land, so a person closes them;
		// a loop epic's done comes only from the server's landing.
		if !epic.IsMap {
			return notify.Error(doneIsSetByServer)
		}
		if unresolved := epic.UnresolvedBlockers(ticket); len(unresolved) > 0 {
			return notify.Error(fmt.Sprintf(
				"%s has unresolved blocked_by (%s); refusing to mark done",
				ticket.Path, strings.Join(unresolved, ", "),
			))
		}
	}
	return cmdApplyTicketStatus(api, file, address, status)
}

// doneIsSetByServer refuses Set done on a loop-epic ticket.
const doneIsSetByServer = "done is set by the server"

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
// panic on the Update goroutine. The write is local; a running server is then
// pinged about address.
func cmdApplyTicketStatus(api ServerAPI, file, address string, status schema.Status) tea.Cmd {
	return func() tea.Msg {
		if err := schema.UpdateTicket(file, func(t *schema.Ticket) {
			t.Status = status
		}); err != nil {
			return notify.Error(err.Error())()
		}
		if api != nil && address != "" {
			_ = api.TicketChanged(context.Background(), address)
		}
		return statusChangedMsg{}
	}
}

// handleStatusChanged applies statusChangedMsg. Up mode does nothing: the
// store watch turns the file write into a ticket-changed event. Down mode has
// no stream, so it reloads from disk.
func (m Model) handleStatusChanged() (tea.Model, tea.Cmd) {
	if m.readsDisk() {
		return m, m.cmdLoadDisk()
	}
	return m, nil
}
