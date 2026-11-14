package ticketmigrate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
)

// BackfillEpicRange stamps range_base and range_tip on every merged loop epic
// under root, live and archived, that has neither. rangeOf answers for one
// epic: ok false stamps nothing (the callback logs why). It is only called for
// those candidates, so a root with none never touches git. It never writes
// merged_at or any other field, and an epic that already has a range is
// skipped, so a rerun is a no-op.
func BackfillEpicRange(root string, rangeOf func(tickets.Epic) (base, tip string, ok bool, err error)) ([]tickets.MigrationChange, error) {
	changes, err := backfillEpicRangeIn(root, rangeOf)
	if err != nil {
		return changes, err
	}
	archive := tickets.ArchiveDir(root)
	if _, err := os.Stat(archive); err != nil {
		return changes, nil
	}
	archived, err := backfillEpicRangeIn(archive, rangeOf)
	return append(changes, archived...), err
}

func backfillEpicRangeIn(dir string, rangeOf func(tickets.Epic) (string, string, bool, error)) ([]tickets.MigrationChange, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var changes []tickets.MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epic := tickets.LoadEpicTicketMD(filepath.Join(dir, entry.Name()))
		epic.Name = entry.Name()
		if !epic.HasTicketMD || epic.IsMap || epic.MergedAt.IsZero() || epic.RangeBase != "" || epic.RangeTip != "" {
			continue
		}
		base, tip, ok, err := rangeOf(epic)
		if err != nil {
			return changes, err
		}
		if !ok {
			continue
		}
		if err := tickets.StampEpicFieldsInner(epic.Path, map[string]any{"range_base": base, "range_tip": tip}); err != nil {
			return changes, err
		}
		changes = append(changes, tickets.MigrationChange{
			Path:  filepath.Join(epic.Path, "ticket.md"),
			Notes: []string{"range_base and range_tip backfilled from the trunk reflog"},
		})
	}
	return changes, nil
}
