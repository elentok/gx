package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GitDirs returns the absolute git dir of dir (for a linked worktree, under
// <common>/worktrees/<name>) and the absolute common git dir. They are equal
// for a plain checkout.
func GitDirs(dir string) (gitDir, commonDir string, err error) {
	out, _, err := run(dir, []string{"rev-parse", "--absolute-git-dir", "--git-common-dir"})
	if err != nil {
		return "", "", fmt.Errorf("%w at %q", ErrNoRepo, dir)
	}
	fields := strings.Split(out, "\n")
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", fmt.Errorf("unexpected rev-parse output %q at %q", out, dir)
	}
	gitDir, commonDir = fields[0], fields[1]
	// --git-common-dir may be relative to dir.
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	return gitDir, filepath.Clean(commonDir), nil
}
