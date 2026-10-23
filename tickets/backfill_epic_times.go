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
		for _, backfill := range []func() (*MigrationChange, error){
			func() (*MigrationChange, error) { return backfillCreatedAt(epicPath, epic) },
			func() (*MigrationChange, error) { return backfillMapCompletedAt(root, entry.Name(), epic) },
		} {
			change, err := backfill()
			if err != nil {
				return changes, err
			}
			if change != nil {
				changes = append(changes, *change)
			}
		}
	}
	return changes, nil
}

// backfillCreatedAt stamps created_at from the file's creation time.
func backfillCreatedAt(epicPath string, epic Epic) (*MigrationChange, error) {
	if !epic.CreatedAt.IsZero() {
		return nil, nil
	}
	created, err := ticketMDCreationTime(epicPath)
	if err != nil {
		return nil, err
	}
	if err := StampEpicFields(epicPath, map[string]any{"created_at": created}); err != nil {
		return nil, err
	}
	return &MigrationChange{
		Path:  filepath.Join(epicPath, "ticket.md"),
		Notes: []string{"created_at backfilled from file creation time"},
	}, nil
}

// backfillMapCompletedAt stamps a map's completed_at from its <map>-impl
// epic, the rule CreateEpic applies when it creates that epic.
func backfillMapCompletedAt(root, name string, epic Epic) (*MigrationChange, error) {
	if !epic.IsMap || !epic.CompletedAt.IsZero() {
		return nil, nil
	}
	implPath := implEpicPath(root, name)
	impl := LoadEpicTicketMD(implPath)
	if !impl.HasTicketMD {
		return nil, nil
	}
	completed := impl.CreatedAt
	if completed.IsZero() {
		var err error
		if completed, err = ticketMDCreationTime(implPath); err != nil {
			return nil, err
		}
	}
	epicPath := filepath.Join(root, name)
	if err := StampEpicFields(epicPath, map[string]any{"completed_at": completed}); err != nil {
		return nil, err
	}
	return &MigrationChange{
		Path:  filepath.Join(epicPath, "ticket.md"),
		Notes: []string{"completed_at backfilled from " + name + ImplEpicSuffix},
	}, nil
}

func ticketMDCreationTime(epicPath string) (time.Time, error) {
	info, err := os.Stat(filepath.Join(epicPath, "ticket.md"))
	if err != nil {
		return time.Time{}, err
	}
	return fileCreationTime(info), nil
}
