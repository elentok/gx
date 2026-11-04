package servertest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	fakeCodexOnce sync.Once
	fakeCodexBin  string // dir holding the built "codex"
	fakeCodexErr  error
)

// FakeCodexDir builds fakecodex once per package run. Tests only read it; the
// package's TestMain calls RemoveFakeCodex after the run.
func FakeCodexDir(t *testing.T) string {
	t.Helper()
	fakeCodexOnce.Do(func() {
		fakeCodexBin, fakeCodexErr = os.MkdirTemp("", "fakecodex-")
		if fakeCodexErr != nil {
			return
		}
		if out, err := exec.Command("go", "build", "-o", filepath.Join(fakeCodexBin, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex").CombinedOutput(); err != nil {
			fakeCodexErr = fmt.Errorf("%w\n%s", err, out)
		}
	})
	if fakeCodexErr != nil {
		t.Fatalf("build fakecodex: %v", fakeCodexErr)
	}
	return fakeCodexBin
}

// RemoveFakeCodex deletes the dir FakeCodexDir built, if any.
func RemoveFakeCodex() {
	if fakeCodexBin != "" {
		os.RemoveAll(fakeCodexBin)
	}
}
