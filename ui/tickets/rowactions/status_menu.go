package rowactions

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// statusChangeCmd writes status to a row's ticket through schema.UpdateTicket,
// mirroring `gx tickets set`'s own guards (cmd/tickets/tickets_set.go) for the two
// transitions that can otherwise corrupt scheduling state: refusing
// status=open on an empty body, and status=done while blocked_by is still
// unresolved. Unlike the CLI's --force, the menu has no override for the done
// guard — a person hand-closing a blocked ticket from the TUI is exactly the
// silent-assertion this ticket exists to prevent.
func statusChangeCmd(tix tabapi.TicketsAPI, store string, epic tickets.Epic, ticket tickets.Ticket, status schema.Status) tea.Cmd {
	file, address := ResolveTicketPath(store, ticket.Path)

	if status == schema.StatusOpen {
		if err := checkTicketBodyBeforeOpen(file); err != nil {
			return notify.Error(err.Error())
		}
	}
	if status == schema.StatusDone {
		// ADR 0030: a map epic's tickets never merge, so a person closes them;
		// a loop epic's done comes only from the server's merge.
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
	return cmdApplyTicketStatus(tix, file, address, status)
}

// doneIsSetByServer refuses Set done on a loop-epic ticket.
const doneIsSetByServer = "done is set by the server"

// checkTicketBodyBeforeOpen mirrors cmd/tickets/tickets_set.go's checkBodyBeforeOpen:
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

// StatusChangedMsg reports that cmdApplyTicketStatus's write finished
// successfully, so updateInner can trigger the same reload cmdRefresh uses.
type StatusChangedMsg struct{}

// cmdApplyTicketStatus performs the validated write in a Cmd (not inline in
// applyStatusChange) so a filesystem error surfaces as a toast rather than a
// panic on the Update goroutine. The write is local; a running server is then
// pinged about address.
func cmdApplyTicketStatus(tix tabapi.TicketsAPI, file, address string, status schema.Status) tea.Cmd {
	return func() tea.Msg {
		if err := schema.UpdateTicket(file, func(t *schema.Ticket) {
			t.Status = status
		}); err != nil {
			return notify.Error(err.Error())()
		}
		if tix != nil && address != "" {
			_ = tix.Changed(context.Background(), address)
		}
		return StatusChangedMsg{}
	}
}
