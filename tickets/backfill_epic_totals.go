package tickets

import "path/filepath"

// BackfillEpicTotals stamps lines_added and lines_removed, and total_tokens and
// actual_cost, on every merged loop epic under root that lacks them. lines
// counts the epic's trailer commits and is only called for an epic missing its
// line counts, so a root with none never touches git; ok false means the epic
// has no repo to count in, and its lines stay unstamped. Tokens and cost are the
// sums SumEpicStats gives at merge: stamped only when every done ticket is.
// Stamping never overwrites, so a rerun changes nothing.
func BackfillEpicTotals(root string, lines func(Epic) (added, removed int, ok bool, err error)) ([]MigrationChange, error) {
	epics, err := Load(root)
	if err != nil {
		return nil, err
	}
	var changes []MigrationChange
	for _, epic := range epics {
		if !epic.HasTicketMD || epic.IsMap || epic.MergedAt.IsZero() {
			continue
		}
		fields := map[string]any{}
		var notes []string
		if epic.LinesAdded == nil || epic.LinesRemoved == nil {
			added, removed, ok, err := lines(epic)
			if err != nil {
				return changes, err
			}
			if ok {
				fields["lines_added"], fields["lines_removed"] = added, removed
				notes = append(notes, "lines backfilled from trailer commits")
			}
		}
		if epic.TotalTokens == nil {
			if stats := SumEpicStats(epic.Tickets); stats.TotalTokens != nil {
				fields["total_tokens"], fields["actual_cost"] = *stats.TotalTokens, stats.ActualCost
				notes = append(notes, "tokens and cost backfilled from ticket stamps")
			}
		}
		if len(fields) == 0 {
			continue
		}
		if err := StampEpicFields(epic.Path, fields); err != nil {
			return changes, err
		}
		changes = append(changes, MigrationChange{
			Path:  filepath.Join(epic.Path, "ticket.md"),
			Notes: notes,
		})
	}
	return changes, nil
}
