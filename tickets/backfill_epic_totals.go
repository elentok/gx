package tickets

import "path/filepath"

// BackfillEpicTotals stamps lines_added and lines_removed, and total_tokens and
// actual_cost, on every merged loop epic of the project at root, archived
// epics included, that lacks them. lines
// counts the epic's trailer commits and is only called for an epic missing its
// line counts, so a root with none never touches git; ok false means the epic
// has no repo to count in, and its lines stay unstamped. Tokens and cost are the
// sums SumEpicStats gives at merge: stamped only when every done ticket is.
// Stamping never overwrites, so a rerun changes nothing.
func BackfillEpicTotals(root string, lines func(Epic) (added, removed int, ok bool, err error)) ([]MigrationChange, error) {
	return backfillEpics(root, func(epic *Epic) ([]MigrationChange, error) {
		if !epic.HasTicketMD || epic.MergedAt.IsZero() {
			return nil, nil
		}
		fields := map[string]any{}
		var notes []string
		if epic.LinesAdded == nil || epic.LinesRemoved == nil {
			added, removed, ok, err := lines(*epic)
			if err != nil {
				return nil, err
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
			return nil, nil
		}
		if err := StampEpicFields(epic.Path, fields); err != nil {
			return nil, err
		}
		return []MigrationChange{{Path: filepath.Join(epic.Path, "ticket.md"), Notes: notes}}, nil
	})
}
