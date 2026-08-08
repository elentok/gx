package git

import "strings"

// TrackedFilesUnder returns the paths (relative to dir) of every git-tracked
// file under relPath, or an empty slice if none are tracked. dir must be a
// real working tree (a plain repo root or a linked worktree) — a bare git
// directory has no index to check against.
func TrackedFilesUnder(dir, relPath string) ([]string, error) {
	out, _, err := run(dir, []string{"ls-files", "--", relPath})
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
