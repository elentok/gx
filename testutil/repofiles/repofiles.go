// Package repofiles lists the repo's source files for the repo-wide guard tests.
package repofiles

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// List returns the repo-relative paths matching pathspec, tracked or not, so a
// new file is scanned before its first `git add`. Ignored files are left out.
// Guard tests run from their package directory, hence the `..`.
func List(t *testing.T, pathspec string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", "..", "ls-files", "--cached", "--others", "--exclude-standard", "--", pathspec)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	return strings.Fields(string(out))
}
