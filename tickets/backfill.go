package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackfillMergedAt stamps merged_at on every loop epic under root that is
// complete, has no merged_at, and for which isMerged says the branch has
// merged. It is how epics that merged before merged_at existed show up as
// merged. A complete epic has completed_at, which merged_at copies; or, when
// it never got one, every ticket finished, and merged_at is the newest
// done_at. isMerged is only called for those candidates, so a root with none
// never touches git. An epic already stamped is skipped, so a rerun is a no-op.
func BackfillMergedAt(root string, isMerged func(Epic) (bool, error)) ([]MigrationChange, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var changes []MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epic := LoadEpicTicketMD(filepath.Join(root, entry.Name()))
		epic.Name = entry.Name()
		if !epic.HasTicketMD || epic.IsMap || !epic.MergedAt.IsZero() {
			continue
		}
		mergedAt, note := epic.CompletedAt, "merged_at backfilled from completed_at"
		if mergedAt.IsZero() {
			epic = loadEpic(root, entry.Name())
			mergedAt, note = lastDoneAt(epic), "merged_at backfilled from the last done_at"
		}
		if mergedAt.IsZero() {
			continue
		}
		ok, err := isMerged(epic)
		if err != nil {
			return changes, err
		}
		if !ok {
			continue
		}
		if err := StampEpicFields(epic.Path, map[string]any{"merged_at": mergedAt}); err != nil {
			return changes, err
		}
		changes = append(changes, MigrationChange{
			Path:  filepath.Join(epic.Path, "ticket.md"),
			Notes: []string{note},
		})
	}
	return changes, nil
}

// lastDoneAt is the newest done_at of a finished epic, or zero when a ticket
// is not done or cancelled, or none has a done_at. It reads the raw status, not
// RenderedStatus: the epic is loaded alone, so a cross-epic blocker would look
// dangling and render a done ticket as an error.
func lastDoneAt(e Epic) time.Time {
	var newest time.Time
	for _, t := range e.Tickets {
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if t.ReadErr != "" || !doneStatuses[status] && !cancelledStatuses[status] {
			return time.Time{}
		}
		if t.DoneAt.After(newest) {
			newest = t.DoneAt
		}
	}
	return newest
}
