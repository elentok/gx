package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackfillEpicTimes stamps created_at on every epic under root that lacks it,
// from its ticket.md's creation time, and completed_at on every map that
// lacks it once its <map>-impl epic exists, from that epic's created_at (its
// file creation time when that isn't stamped yet). Stamping never overwrites,
// so a rerun is a no-op.
func BackfillEpicTimes(root string) ([]MigrationChange, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var changes []MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicPath := filepath.Join(root, entry.Name())
		epic := LoadEpicTicketMD(epicPath)
		if !epic.HasTicketMD {
			continue
		}
		if epic.CreatedAt.IsZero() {
			created, err := ticketMDCreationTime(epicPath)
			if err != nil {
				return changes, err
			}
			if err := StampEpicFields(epicPath, map[string]any{"created_at": created}); err != nil {
				return changes, err
			}
			changes = append(changes, MigrationChange{
				Path:  filepath.Join(epicPath, "ticket.md"),
				Notes: []string{"created_at backfilled from file creation time"},
			})
		}
		if !epic.IsMap || !epic.CompletedAt.IsZero() {
			continue
		}
		implPath := filepath.Join(root, entry.Name()+ImplEpicSuffix)
		impl := LoadEpicTicketMD(implPath)
		if !impl.HasTicketMD {
			continue
		}
		completed := impl.CreatedAt
		if completed.IsZero() {
			if completed, err = ticketMDCreationTime(implPath); err != nil {
				return changes, err
			}
		}
		if err := StampEpicFields(epicPath, map[string]any{"completed_at": completed}); err != nil {
			return changes, err
		}
		changes = append(changes, MigrationChange{
			Path:  filepath.Join(epicPath, "ticket.md"),
			Notes: []string{"completed_at backfilled from " + entry.Name() + ImplEpicSuffix},
		})
	}
	return changes, nil
}

func ticketMDCreationTime(epicPath string) (time.Time, error) {
	info, err := os.Stat(filepath.Join(epicPath, "ticket.md"))
	if err != nil {
		return time.Time{}, err
	}
	return fileCreationTime(info), nil
}
