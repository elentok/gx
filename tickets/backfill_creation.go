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
	return backfillFromHistory(epics, storeRoot, origin, dryRun, backfillSpec{
		skip: func(t Ticket) bool { return !t.CreatedAt.IsZero() && t.OriginalAddress != "" },
		plan: func(t Ticket, o TicketOrigin) (notes []string, apply func(*schema.Ticket)) {
			original, hasOriginal := originalAddressOf(o.Path)
			if t.CreatedAt.IsZero() {
				notes = append(notes, "created_at backfilled from store history")
			}
			if t.OriginalAddress == "" && hasOriginal {
				notes = append(notes, "original address backfilled from store history")
			}
			return notes, func(st *schema.Ticket) {
				if st.CreatedAt.IsZero() {
					st.CreatedAt = o.day()
				}
				if st.OriginalAddress == "" && hasOriginal {
					st.OriginalAddress = original
				}
			}
		},
	})
}

// BackfillCancelledAt stamps cancelled_at on every cancelled ticket of the
// given epics that lacks it, from the store commit that set status: cancelled.
// cancelledIn reports that commit by the ticket's path relative to storeRoot;
// ok false means no history, and the ticket is left alone. A ticket already
// cancelled in the store's root commit gets that commit's day, as in
// BackfillTicketCreation. Only a zero cancelled_at is stamped, so a rerun
// changes nothing. With dryRun it reports without writing.
func BackfillCancelledAt(
	epics []Epic,
	storeRoot string,
	cancelledIn func(rel string) (TicketOrigin, bool, error),
	dryRun bool,
) ([]MigrationChange, error) {
	return backfillFromHistory(epics, storeRoot, cancelledIn, dryRun, backfillSpec{
		skip: func(t Ticket) bool { return !t.IsCancelled() || !t.CancelledAt.IsZero() },
		plan: func(_ Ticket, o TicketOrigin) ([]string, func(*schema.Ticket)) {
			return []string{"cancelled_at backfilled from store history"}, func(st *schema.Ticket) {
				if st.CancelledAt.IsZero() {
					st.CancelledAt = o.day()
				}
			}
		},
	})
}

// day is the commit's time, or for the store's root commit, which predates the
// store, only its day.
func (o TicketOrigin) day() time.Time {
	if o.Root {
		return o.At.UTC().Truncate(24 * time.Hour)
	}
	return o.At
}

// backfillSpec is what differs between the history backfills. skip passes over
// tickets that need nothing. plan returns the notes to report (none means
// nothing to do) and the stamp to write unless dryRun.
type backfillSpec struct {
	skip func(Ticket) bool
	plan func(Ticket, TicketOrigin) ([]string, func(*schema.Ticket))
}

// backfillFromHistory is the shared walk of the two history backfills: for each
// ticket spec does not skip, it looks up the ticket's store history and asks
// spec.plan what to stamp.
func backfillFromHistory(
	epics []Epic,
	storeRoot string,
	lookup func(rel string) (TicketOrigin, bool, error),
	dryRun bool,
	spec backfillSpec,
) ([]MigrationChange, error) {
	var changes []MigrationChange
	for _, epic := range epics {
		for _, t := range epic.Tickets {
			if spec.skip(t) {
				continue
			}
			rel, err := filepath.Rel(storeRoot, t.FilePath())
			if err != nil {
				return changes, err
			}
			o, ok, err := lookup(filepath.ToSlash(rel))
			if err != nil {
				return changes, fmt.Errorf("history of %s: %w", t.FilePath(), err)
			}
			if !ok {
				continue
			}
			notes, stamp := spec.plan(t, o)
			if len(notes) == 0 {
				continue
			}
			if !dryRun {
				if err := schema.UpdateTicket(t.FilePath(), stamp); err != nil {
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
