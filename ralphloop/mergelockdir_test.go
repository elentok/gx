package ralphloop

import (
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop/deps"
)

func TestDefaultDepsWithOverrides_NoStateDirFailsMergeLockDir(t *testing.T) {
	t.Parallel()
	d := deps.DefaultDepsWithOverrides(deps.DepsOverrides{})
	if _, err := mergeLockDirFor(d, "/tickets/proj", "epic"); err == nil {
		t.Error("mergeLockDirFor() with no state dir = nil error, want one")
	}
	state := t.TempDir()
	d = deps.DefaultDepsWithOverrides(deps.DepsOverrides{StateDir: state})
	dir, err := mergeLockDirFor(d, "/tickets/proj", "epic")
	if err != nil || !strings.HasPrefix(dir, state) {
		t.Errorf("mergeLockDirFor() = %q, %v; want a dir under %s", dir, err, state)
	}
}
