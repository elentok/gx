package nativerunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// builtBinary builds one Go package once per test run. Tests share the result
// and must not modify it.
type builtBinary struct {
	pkg, name string
	once      sync.Once
	path      string
	err       error
}

// builds counts real builds, so a test can assert each binary is built once.
var builds atomic.Int32

var (
	builtGx         = &builtBinary{pkg: "github.com/elentok/gx", name: "gx"}
	builtFakeclaude = &builtBinary{pkg: "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude", name: "claude"}
	builtFakecodex  = &builtBinary{pkg: "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex", name: "codex"}
)

// get returns the binary's path. It is named name inside a directory of its own,
// so the directory can go on PATH.
func (b *builtBinary) get(t *testing.T) string {
	t.Helper()
	b.once.Do(func() {
		dir, err := os.MkdirTemp("", "nativerunner-"+b.name)
		if err != nil {
			b.err = err
			return
		}
		b.path = filepath.Join(dir, b.name)
		builds.Add(1)
		if out, err := exec.Command("go", "build", "-o", b.path, b.pkg).CombinedOutput(); err != nil {
			b.err = &buildError{b.pkg, err, string(out)}
		}
	})
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b.path
}

type buildError struct {
	pkg string
	err error
	out string
}

func (e *buildError) Error() string { return "build " + e.pkg + ": " + e.err.Error() + "\n" + e.out }

// TestBuiltBinaries_BuiltOncePerRun asserts after two gets that each binary
// was built at most once, whatever the other tests already asked for.
func TestBuiltBinaries_BuiltOncePerRun(t *testing.T) {
	bins := []*builtBinary{builtFakeclaude, builtFakecodex}
	for _, b := range bins {
		first := b.get(t)
		before := builds.Load()
		if second := b.get(t); second != first {
			t.Fatalf("%s: path changed %q -> %q", b.name, first, second)
		}
		if builds.Load() != before {
			t.Fatalf("%s: second get built again", b.name)
		}
	}
	if n := builds.Load(); n > 3 {
		t.Fatalf("builds = %d, want at most 3 (gx, fakeclaude, fakecodex)", n)
	}
}
