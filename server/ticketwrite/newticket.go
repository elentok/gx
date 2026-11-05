package ticketwrite

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
)

// WriteNewTicket allocates the next id under the epic lock (a child of parent
// when parent is set) and writes the ticket build returns for it.
func WriteNewTicket(epicPath, parent, slug string, build func(id string) (schema.Ticket, string)) (path, id string, err error) {
	epic, unlock, err := ticketmove.LoadLockedEpic(epicPath)
	if err != nil {
		return "", "", fmt.Errorf("load epic %s: %w", epicPath, err)
	}
	defer unlock()

	id, err = ticketmove.NextTicketID(*epic, parent)
	if err != nil {
		return "", "", fmt.Errorf("allocate %s id: %w", slug, err)
	}
	tk, body := build(id)
	tk.StampCreated(time.Now(), tickets.Address{Project: tickets.ProjectName(filepath.Dir(epicPath)), Epic: filepath.Base(epicPath), ID: id}.String())
	out, err := schema.MarshalTicket(tk, body)
	if err != nil {
		return "", "", fmt.Errorf("marshal %s ticket %s: %w", slug, id, err)
	}
	path = filepath.Join(epicPath, "issues", fmt.Sprintf("%s-%s.md", id, slug))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := schema.ParseTicket(path); err != nil {
		return "", "", fmt.Errorf("%s ticket %s failed validation: %w", slug, path, err)
	}
	return path, id, nil
}
