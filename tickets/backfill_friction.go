package tickets

import (
	"path/filepath"

	"github.com/elentok/gx/tickets/schema"
)

// BackfillFriction stamps a Friction summary on every done or cancelled ticket
// of the project at root, archived epics included, that lacks one, then on every
// merged loop epic that lacks one, as the sum of its done tickets' summaries.
// friction builds a ticket's summary from its epic's run log and its own log.
// Only a missing summary is stamped, so a rerun changes nothing.
func BackfillFriction(root string, friction func(epicName string, t Ticket) (schema.FrictionSummary, error)) ([]MigrationChange, error) {
	return backfillEpics(root, func(epic *Epic) ([]MigrationChange, error) {
		var changes []MigrationChange
		for i := range epic.Tickets {
			t := &epic.Tickets[i]
			if !(t.IsDone() || t.IsCancelled()) || t.Friction != nil {
				continue
			}
			f, err := friction(epic.Name, *t)
			if err != nil {
				return changes, err
			}
			if err := schema.UpdateTicket(t.FilePath(), func(st *schema.Ticket) { st.Friction = &f }); err != nil {
				return changes, err
			}
			t.Friction = &f
			changes = append(changes, MigrationChange{Path: t.FilePath(), Notes: []string{"friction summary backfilled"}})
		}

		if !epic.HasTicketMD || epic.MergedAt.IsZero() || epic.Friction != nil {
			return changes, nil
		}
		if err := StampEpicFields(epic.Path, map[string]any{"friction": SumEpicStats(epic.Tickets).Friction}); err != nil {
			return changes, err
		}
		return append(changes, MigrationChange{
			Path:  filepath.Join(epic.Path, "ticket.md"),
			Notes: []string{"friction summary backfilled from ticket summaries"},
		}), nil
	})
}
