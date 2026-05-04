package commit

import "github.com/elentok/gx/git"

func (m *Model) moveToAdjacentCommit(delta int) bool {
	if delta == 0 || m.details.FullHash == "" {
		return false
	}
	entries, err := git.LogEntries(m.worktreeRoot, "HEAD", 250)
	if err != nil || len(entries) == 0 {
		return false
	}
	idx := indexOfCommit(entries, m.details.FullHash)
	if idx < 0 {
		return false
	}
	next := idx + delta
	if next < 0 || next >= len(entries) {
		return false
	}
	m.ref = entries[next].FullHash
	m.reload()
	return true
}

func indexOfCommit(entries []git.LogEntry, fullHash string) int {
	for i, entry := range entries {
		if entry.FullHash == fullHash {
			return i
		}
	}
	return -1
}
