package tickets

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/tickets/schema"
)

// ErrUnreadable marks a session transcript that is missing, ambiguous or cannot
// be parsed. BackfillTicketTotals skips such a ticket; any other error from
// totals aborts the backfill.
var ErrUnreadable = errors.New("unreadable transcript")

// BackfillTicketTotals stamps total_tokens and actual_cost on every done
// ticket under root that lacks total_tokens, so a merged epic's sums can use
// them. totals sums the sessions' tokens and cost and fails with ErrUnreadable
// unless every transcript can be read; such a ticket stays as it is, old
// actual_cost included, and is reported as skipped. conflictSessions gives the sessions the run log recorded for the
// conflicts of an epic's ticket: a conflict-resolution child never recorded its
// own, so it takes its parent's. A ticket with no sessions stamps zero only when
// it is commitless or was done by hand (never claimed), and stays unstamped
// otherwise. Only a missing total_tokens is stamped, so a rerun changes
// nothing.
func BackfillTicketTotals(
	root string,
	totals func(ids []string) (tokens int64, cost float64, err error),
	conflictSessions func(epicName, ticketID string) ([]string, error),
) ([]MigrationChange, error) {
	return backfillEpics(root, func(epic *Epic) ([]MigrationChange, error) {
		var changes []MigrationChange
		for i := range epic.Tickets {
			t := &epic.Tickets[i]
			if !t.IsDone() || t.TotalTokens != nil {
				continue
			}
			st, err := schema.ParseTicket(t.FilePath())
			if err != nil {
				return changes, err
			}
			ids := slices.Clone(st.SessionIDs)
			if len(ids) == 0 && st.Type == schema.TypeConflictResolution && st.Parent != nil {
				if ids, err = conflictSessions(epic.Name, string(*st.Parent)); err != nil {
					return changes, fmt.Errorf("run log of %s: %w", epic.Name, err)
				}
			}
			var tokens int64
			var cost float64
			if len(ids) == 0 {
				if !st.IsCommitless() && !st.ClaimedAt.IsZero() {
					continue
				}
			} else if tokens, cost, err = totals(ids); errors.Is(err, ErrUnreadable) {
				changes = append(changes, MigrationChange{
					Path:  t.FilePath(),
					Notes: []string{"skipped: unreadable transcript"},
				})
				continue
			} else if err != nil {
				return changes, fmt.Errorf("totals of %s: %w", t.FilePath(), err)
			}
			err = schema.UpdateTicket(t.FilePath(), func(st *schema.Ticket) {
				st.TotalTokens = &tokens
				st.ActualCost = cost
			})
			if err != nil {
				return changes, err
			}
			t.TotalTokens, t.ActualCost = &tokens, cost
			changes = append(changes, MigrationChange{
				Path:  t.FilePath(),
				Notes: []string{"tokens and cost backfilled from transcripts"},
			})
		}
		return changes, nil
	})
}
