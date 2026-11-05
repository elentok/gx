package ticketmigrate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// MarkMapDone is the rule that a map is done once its <map>-impl epic has
// merged: it copies the impl epic's merged_at onto the map and sets the map's
// status to done. implPath is the impl epic's directory. An epic that is not an
// <map>-impl, has not merged, or whose map is missing or already done changes
// nothing. With dryRun it reports the change without writing.
func MarkMapDone(implPath string, dryRun bool) (*tickets.MigrationChange, error) {
	mapName, ok := strings.CutSuffix(filepath.Base(implPath), tickets.ImplEpicSuffix)
	if !ok {
		return nil, nil
	}
	impl := tickets.LoadEpicTicketMD(implPath)
	if !impl.HasTicketMD || impl.MergedAt.IsZero() {
		return nil, nil
	}
	mapPath := filepath.Join(filepath.Dir(implPath), mapName)
	m := tickets.LoadEpicTicketMD(mapPath)
	if !m.HasTicketMD || !m.IsMap || (m.Status == string(schema.StatusDone) && !m.MergedAt.IsZero()) {
		return nil, nil
	}
	if !dryRun {
		if err := tickets.StampEpicFieldsInner(mapPath, map[string]any{"merged_at": impl.MergedAt}); err != nil {
			return nil, err
		}
		if err := tickets.OverwriteEpicFieldsInner(mapPath, map[string]any{"status": string(schema.StatusDone)}); err != nil {
			return nil, err
		}
	}
	return &tickets.MigrationChange{
		Path:  filepath.Join(mapPath, "ticket.md"),
		Notes: []string{"merged_at copied from " + mapName + tickets.ImplEpicSuffix + ", status done"},
	}, nil
}

// MigrateMapDone applies MarkMapDone to every <map>-impl epic under root. A
// rerun changes nothing.
func MigrateMapDone(root string, dryRun bool) ([]tickets.MigrationChange, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var changes []tickets.MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		change, err := MarkMapDone(filepath.Join(root, entry.Name()), dryRun)
		if err != nil {
			return changes, err
		}
		if change != nil {
			changes = append(changes, *change)
		}
	}
	return changes, nil
}
