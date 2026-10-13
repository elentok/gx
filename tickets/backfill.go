package tickets

import (
	"os"
	"path/filepath"
	"strings"
)

// BackfillMergedAt stamps merged_at = completed_at on every loop epic under
// root that has completed_at and no merged_at, and whose branch landed says
// has landed. It is how epics that landed before merged_at existed show up as
// merged. landed is only called for those candidates, so a root with none
// never touches git. An epic already stamped is skipped, so a rerun is a no-op.
func BackfillMergedAt(root string, landed func(Epic) (bool, error)) ([]MigrationChange, error) {
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
		if !epic.HasTicketMD || epic.IsMap || epic.CompletedAt.IsZero() || !epic.MergedAt.IsZero() {
			continue
		}
		ok, err := landed(epic)
		if err != nil {
			return changes, err
		}
		if !ok {
			continue
		}
		if err := StampEpicFields(epic.Path, map[string]any{"merged_at": epic.CompletedAt}); err != nil {
			return changes, err
		}
		changes = append(changes, MigrationChange{
			Path:  filepath.Join(epic.Path, "ticket.md"),
			Notes: []string{"merged_at backfilled from completed_at"},
		})
	}
	return changes, nil
}
