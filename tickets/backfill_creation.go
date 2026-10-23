package tickets

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// TicketOrigin is where a ticket file first appeared in the ticket store's
// history: its path then (relative to the store root), when, and whether that
// was the store's first commit.
type TicketOrigin struct {
	Path string
	At   time.Time
	Root bool
}

// BackfillTicketCreation stamps created_at and the original address on every
// ticket of the given epics that lacks them, from the ticket store's history.
// origin reports a file's first commit by its path relative to storeRoot; ok
// false means it has no history, and the ticket is left alone. A ticket first
// committed in the store's root commit predates the store, so it gets that
// commit's day rather than a time that means nothing. Only zero fields are
// stamped, so a rerun changes nothing. With dryRun it reports without writing.
func BackfillTicketCreation(
	epics []Epic,
	storeRoot string,
	origin func(rel string) (TicketOrigin, bool, error),
	dryRun bool,
) ([]MigrationChange, error) {
	var changes []MigrationChange
	for _, epic := range epics {
		for _, t := range epic.Tickets {
			if !t.CreatedAt.IsZero() && t.OriginalAddress != "" {
				continue
			}
			rel, err := filepath.Rel(storeRoot, t.FilePath())
			if err != nil {
				return changes, err
			}
			o, ok, err := origin(filepath.ToSlash(rel))
			if err != nil {
				return changes, fmt.Errorf("history of %s: %w", t.FilePath(), err)
			}
			if !ok {
				continue
			}
			created := o.At
			if o.Root {
				created = o.At.UTC().Truncate(24 * time.Hour)
			}
			original, hasOriginal := originalAddressOf(o.Path)

			var notes []string
			if t.CreatedAt.IsZero() {
				notes = append(notes, "created_at backfilled from store history")
			}
			if t.OriginalAddress == "" && hasOriginal {
				notes = append(notes, "original address backfilled from store history")
			}
			if len(notes) == 0 {
				continue
			}
			if !dryRun {
				err = schema.UpdateTicket(t.FilePath(), func(st *schema.Ticket) {
					if st.CreatedAt.IsZero() {
						st.CreatedAt = created
					}
					if st.OriginalAddress == "" && hasOriginal {
						st.OriginalAddress = original
					}
				})
				if err != nil {
					return changes, err
				}
			}
			changes = append(changes, MigrationChange{Path: t.FilePath(), Notes: notes})
		}
	}
	return changes, nil
}

// originalAddressOf is the address of the ticket at a store path of the form
// <project>/[.archive/]<epic>/issues/<file>. An archive directory is not part
// of the address.
func originalAddressOf(storePath string) (string, bool) {
	parts := strings.Split(storePath, "/")
	if len(parts) >= 2 && parts[1] == ".archive" {
		parts = append(parts[:1], parts[2:]...)
	}
	if len(parts) != 4 || parts[2] != "issues" {
		return "", false
	}
	_, id, _, ok := parseTicketFilename(parts[3])
	if !ok {
		return "", false
	}
	return Address{Project: parts[0], Epic: parts[1], ID: id}.String(), true
}
