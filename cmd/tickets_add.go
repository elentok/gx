package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsAdd atomically allocates the next ticket ID for epicPath (a
// flat sibling with no parent, a lettered child of parent, or the next
// number under a lettered parent regardless of whether parent itself
// already carries trailing digits — see tickets.NextTicketID) and writes a
// minimal stub ticket file for the caller to fill in, printing the created
// file's path to w. slug must be non-empty — it becomes the stub's
// filename suffix (<id>-<slug>.md), so the file lands with a real name
// instead of a placeholder the caller has to remember to rename.
func runTicketsAdd(epicPath, parent, slug string, w io.Writer) error {
	if slug == "" {
		return errors.New("slug is required")
	}

	epicPath = filepath.Clean(epicPath)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return err
	}
	defer unlock()

	id, err := tickets.NextTicketID(*epic, parent)
	if err != nil {
		return err
	}

	stubPath := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-%s.md", id, slug))
	stub := schema.Ticket{
		ID:     schema.TicketID(id),
		Status: schema.StatusOpen,
		Type:   schema.TypeTask,
	}
	if parent != "" {
		parentID := schema.TicketID(parent)
		stub.Parent = &parentID
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

	// Best-effort backfill of the parent's own children: fullyDone
	// (tickets/status.go) derives a ticket's descendants from Parent
	// pointers, not this field, so a failure here never breaks blocked_by
	// resolution — it only keeps children accurate for humans/UI that read
	// it directly. Never let any other code path start trusting children as
	// authoritative again (see Epic.childrenIndex's doc comment).
	if parent != "" {
		if err := appendChild(*epic, parent, id); err != nil {
			fmt.Fprintf(w, "warning: created %s but failed to backfill %s's children: %v\n", id, parent, err)
		}
	}

	fmt.Fprintln(w, stubPath)
	return nil
}

// appendChild adds childID to parentID's own children list within epic, a
// one-time write at fork-creation time so children stays in sync with the
// Parent field this same call just wrote on the new ticket. Idempotent: a
// childID already present is left alone.
func appendChild(epic tickets.Epic, parentID, childID string) error {
	var parentPath string
	for _, t := range epic.Tickets {
		if t.Identifier == parentID {
			parentPath = t.Path
			break
		}
	}
	if parentPath == "" {
		return fmt.Errorf("parent ticket %s not found in epic", parentID)
	}
	return schema.UpdateTicket(parentPath, func(t *schema.Ticket) {
		child := schema.TicketID(childID)
		for _, existing := range t.Children {
			if existing == child {
				return
			}
		}
		t.Children = append(t.Children, child)
	})
}
