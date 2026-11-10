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

// archiveEpicPrefix starts the epic part of an archive key. Archive names
// never start with a dot, so no active epic can collide with it.
const archiveEpicPrefix = ".archive/"

// ArchiveKey re-keys addr (an epic or ticket address) to the archive-only key
// project:.archive/<archiveName>, keeping a /NN ticket suffix. Only the epic
// part changes, so a new active epic that reuses the name never matches it.
func ArchiveKey(addr, archiveName string) string {
	epicPart, suffix, hasSuffix := strings.Cut(addr, "/")
	project, _, _ := strings.Cut(epicPart, ":")
	key := project + ":" + archiveEpicPrefix + archiveName
	if hasSuffix {
		key += "/" + suffix
	}
	return key
}

// IsArchiveKey reports whether s is an archive key (see ArchiveKey).
func IsArchiveKey(s string) bool {
	_, ok := ArchiveKeyAddress(s)
	return ok
}

// ArchiveKeyAddress reads the address back from an archive key: the archive
// name becomes the epic name and a /NN suffix is kept. It reports false for
// anything that is not an archive key.
func ArchiveKeyAddress(key string) (string, bool) {
	project, rest, ok := strings.Cut(key, ":")
	if !ok || project == "" {
		return "", false
	}
	rest, ok = strings.CutPrefix(rest, archiveEpicPrefix)
	if !ok || rest == "" || strings.HasPrefix(rest, "/") {
		return "", false
	}
	return project + ":" + rest, true
}

// ArchiveEpicKey drops a /NN ticket suffix from an archive key, leaving the
// epic's key. EpicAddress cannot do this: it cuts at the first slash, which
// inside an archive key is the one after ".archive".
func ArchiveEpicKey(key string) string {
	head, rest, ok := strings.Cut(key, archiveEpicPrefix)
	if !ok {
		return key
	}
	name, _, _ := strings.Cut(rest, "/")
	return head + archiveEpicPrefix + name
}

// CountArchivedEpics reports how many epics live under scratchDir's
// ".archive" directory via a flat, non-recursive directory listing — no
// ticket-file parsing. It's cheap enough to call unconditionally (e.g. every
// Tickets tab load and auto-refresh tick). A missing ".archive" directory is
// not an error: it reports zero, mirroring how Load handles a missing
// ".scratch/".
func CountArchivedEpics(scratchDir string) (int, error) {
	names, err := ArchivedEpicNames(scratchDir)
	return len(names), err
}

// ArchivedEpicNames is the set of epic names under scratchDir's ".archive"
// directory, from the same flat listing as CountArchivedEpics. A missing
// directory is an empty set.
func ArchivedEpicNames(scratchDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(ArchiveDir(scratchDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names[entry.Name()] = true
		}
	}
	return names, nil
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
