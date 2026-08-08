package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsAdd atomically allocates the next ticket ID for epicPath (a
// flat sibling with no parent, a lettered child of parent, or one numeric
// level past a lettered parent — see tickets.NextTicketID) and writes a
// minimal stub ticket file for the caller to fill in, printing the created
// file's path to w.
func runTicketsAdd(epicPath, parent string, w io.Writer) error {
	epicPath = filepath.Clean(epicPath)
	scratchDir := filepath.Dir(epicPath)
	epicName := filepath.Base(epicPath)

	if info, err := os.Stat(epicPath); err != nil || !info.IsDir() {
		return fmt.Errorf("epic not found: %s", epicPath)
	}

	unlock, err := tickets.LockEpic(epicPath)
	if err != nil {
		return err
	}
	defer unlock()

	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return fmt.Errorf("loading epics under %s: %w", scratchDir, err)
	}
	var epic *tickets.Epic
	for i := range epics {
		if epics[i].Name == epicName {
			epic = &epics[i]
			break
		}
	}
	if epic == nil {
		return fmt.Errorf("epic not found: %s", epicPath)
	}

	id, err := tickets.NextTicketID(*epic, parent)
	if err != nil {
		return err
	}

	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-new-ticket.md", id))
	stub := schema.Ticket{
		ID:     schema.TicketID(id),
		Status: schema.StatusOpen,
		Type:   schema.TypeTask,
	}
	body := fmt.Sprintf(
		"\n# %s\n\n## What to build\n\n\n## Test seams\n\n\n## Acceptance criteria\n\n- [ ] \n",
		id,
	)

	out, err := schema.MarshalTicket(stub, body)
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

	fmt.Fprintln(w, stubPath)
	return nil
}
