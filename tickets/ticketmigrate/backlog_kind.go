package ticketmigrate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elentok/gx/tickets"
)

// legacyAreaKind matches the `kind: area` line of an epic ticket.md front
// matter, the kind a backlog epic had before it was renamed.
var legacyAreaKind = regexp.MustCompile(`(?m)^kind:[ \t]*"?area"?[ \t]*$`)

// MigrateAreaToBacklogKind rewrites `kind: area` to `kind: backlog` in the epic
// ticket.md files under root and its archive. A rerun changes nothing.
func MigrateAreaToBacklogKind(root string) ([]tickets.MigrationChange, error) {
	changes, err := areaToBacklogIn(root)
	if err != nil {
		return changes, err
	}
	archive := tickets.ArchiveDir(root)
	if _, err := os.Stat(archive); err != nil {
		return changes, nil
	}
	archived, err := areaToBacklogIn(archive)
	return append(changes, archived...), err
}

func areaToBacklogIn(dir string) ([]tickets.MigrationChange, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var changes []tickets.MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicDir := filepath.Join(dir, entry.Name())
		path := filepath.Join(epicDir, "ticket.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !legacyAreaKind.Match(frontMatter(data)) {
			continue
		}
		if err := tickets.OverwriteEpicFieldsInner(epicDir, map[string]any{"kind": tickets.KindBacklog}); err != nil {
			return changes, err
		}
		changes = append(changes, tickets.MigrationChange{Path: path, Notes: []string{"kind area renamed to backlog"}})
	}
	return changes, nil
}

// frontMatter returns the text between the leading "---" lines, or nil.
func frontMatter(data []byte) []byte {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return nil
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return nil
	}
	return []byte(s[4 : 4+end])
}
