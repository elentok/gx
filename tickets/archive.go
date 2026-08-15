package tickets

import (
	"os"
	"path/filepath"
	"strings"
)

// ArchiveDir returns scratchDir's ".archive" subdirectory, the filesystem
// convention (see the gx-cleanup skill) for a fully-merged, fully-done epic
// moved out of the active tracker.
func ArchiveDir(scratchDir string) string {
	return filepath.Join(scratchDir, ".archive")
}

// CountArchivedEpics reports how many epics live under scratchDir's
// ".archive" directory via a flat, non-recursive directory listing — no
// ticket-file parsing. It's cheap enough to call unconditionally (e.g. every
// Tickets tab load and auto-refresh tick). A missing ".archive" directory is
// not an error: it reports zero, mirroring how Load handles a missing
// ".scratch/".
func CountArchivedEpics(scratchDir string) (int, error) {
	entries, err := os.ReadDir(ArchiveDir(scratchDir))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			count++
		}
	}
	return count, nil
}

// LoadArchived walks scratchDir's ".archive" directory the same way Load
// walks ".scratch/", reusing the same per-epic parsing (including malformed/
// unreadable ticket file handling). It's a separate entry point from Load,
// not a flag on it, since callers load it lazily and on a different cadence
// than the active tracker. A missing ".archive" directory is not an error:
// it returns a nil/empty slice, matching Load's handling of a missing
// ".scratch/".
func LoadArchived(scratchDir string) ([]Epic, error) {
	return Load(ArchiveDir(scratchDir))
}
