package history

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/internal/run"
)

// ReflogEntry is one update of a branch: the tip before (Old) and after (New).
type ReflogEntry struct{ Old, New string }

const zeroSHA = "0000000000000000000000000000000000000000"

// ReflogEntries lists, oldest first, the updates of branch target that moved
// it forward: Old is a proper ancestor of New, which covers fast-forwards and
// merge commits. Resets, amends and the branch creation (zero Old) are
// skipped. It reads <common dir>/logs/refs/heads/<target> because `git reflog
// show` prints only the new SHA. A missing log is an empty list, not an error.
func ReflogEntries(dir, target string) ([]ReflogEntry, error) {
	_, commonDir, err := git.GitDirs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(commonDir, "logs", "refs", "heads", filepath.FromSlash(target)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []ReflogEntry
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		old, next := fields[0], fields[1]
		if old == zeroSHA || old == next {
			continue
		}
		ok, err := git.IsAncestor(dir, old, next)
		if err != nil {
			return nil, err
		}
		if ok {
			entries = append(entries, ReflogEntry{Old: old, New: next})
		}
	}
	return entries, nil
}

// TrailerCommit is a commit with every value of one trailer key. A squash
// body can carry several lines of the same key.
type TrailerCommit struct {
	SHA    string
	Values []string
}

// TrailerCommitsIn lists every commit of rev (a ref or a range) that has a
// key trailer, with all its trailer values.
func TrailerCommitsIn(dir, rev, key string) ([]TrailerCommit, error) {
	out, _, err := run.Exec(dir, []string{"log", "--format=%x00%H%n%(trailers:key=" + key + ",valueonly)", rev, "--"})
	if err != nil {
		return nil, err
	}
	var result []TrailerCommit
	for record := range strings.SplitSeq(out, "\x00") {
		lines := strings.Split(record, "\n")
		sha := strings.TrimSpace(lines[0])
		if sha == "" {
			continue
		}
		var values []string
		for _, l := range lines[1:] {
			if l = strings.TrimSpace(l); l != "" {
				values = append(values, l)
			}
		}
		if len(values) > 0 {
			result = append(result, TrailerCommit{SHA: sha, Values: values})
		}
	}
	return result, nil
}
