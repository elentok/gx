package testutil

import (
	"fmt"
	"os"
	"strings"
)

// holderSocketBudget is how many bytes below a test root the deepest holder
// socket path may take ("/data/gx/agents/<id>/holder.sock" with room to spare).
// A unix socket path is capped near 100 bytes, so the root must leave this much.
const holderSocketBudget = 60

// mkdirTempSuffix is the random suffix os.MkdirTemp appends to the pattern.
const mkdirTempSuffix = 10

const maxSocketPath = 100

// ShortTempDir makes a temp dir short enough that a holder socket below it
// still binds: under $TMPDIR when that leaves holderSocketBudget bytes, else
// under /tmp (macOS's TMPDIR is too long). The caller removes it. A failure
// names the directory it could not write to.
func ShortTempDir(pattern string) (string, error) {
	base := os.Getenv("TMPDIR")
	if base == "" {
		base = os.TempDir()
	}
	base = strings.TrimRight(base, "/")
	if len(base)+1+len(pattern)+mkdirTempSuffix+holderSocketBudget > maxSocketPath {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, pattern)
	if err != nil {
		return "", fmt.Errorf("short test root: cannot create a temp dir under %s (it must be writable; the holder socket path needs a short root): %w", base, err)
	}
	return dir, nil
}
