package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// typedTicketSpec is what differs between the epic's one-per-epic ticket
// types. name doubles as the filename suffix and the label in messages;
// bodyFormat takes the ticket id and the epic name, then bodyArgs.
type typedTicketSpec struct {
	ticketType schema.TicketType
	name       string
	bodyFormat string
	bodyArgs   []any

	contextWindow int
}

// ensureTypedTicket is a no-op when epicPath already has a ticket of
// spec.ticketType, else it stamps out an open stub. The epic lock is held
// across the check and the write so two callers can't both add one.
func ensureTypedTicket(epicPath string, w io.Writer, spec typedTicketSpec) error {
	epicPath = filepath.Clean(epicPath)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return err
	}
	defer unlock()

	for _, t := range epic.Tickets {
		if t.Type == string(spec.ticketType) {
			fmt.Fprintf(w, "%s: already has a %s ticket (%s)\n", epicPath, spec.name, t.DisplayNumber())
			return nil
		}
	}

	id, err := tickets.NextTicketID(*epic, "")
	if err != nil {
		return err
	}
	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-%s.md", id, spec.name))

	stub := schema.Ticket{
		ID:                    schema.TicketID(id),
		Status:                schema.StatusOpen,
		Type:                  spec.ticketType,
		ExpectedContextWindow: spec.contextWindow,
	}
	address, ok := tickets.AddressOfPath(stubPath, id)
	if !ok {
		return fmt.Errorf("no address for new ticket %s", stubPath)
	}
	stub.StampCreated(time.Now().UTC(), address.String())

	out, err := schema.MarshalTicket(stub, fmt.Sprintf(spec.bodyFormat, append([]any{id, epic.Name}, spec.bodyArgs...)...))
	if err != nil {
		return fmt.Errorf("marshaling stub ticket: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(stubPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(stubPath, out, 0644); err != nil {
		return fmt.Errorf("writing stub ticket %s: %w", stubPath, err)
	}

	if _, err := schema.ParseTicket(stubPath); err != nil {
		return fmt.Errorf("stub ticket %s failed validation: %w", stubPath, err)
	}

	fmt.Fprintf(w, "%s: created %s stub\n", stubPath, spec.name)
	return nil
}
