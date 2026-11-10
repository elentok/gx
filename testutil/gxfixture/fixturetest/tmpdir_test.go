package fixturetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSweepOrphans_RemovesDeadRunsAndKeepsLiveOnes(t *testing.T) {
	root := t.TempDir()
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	orphan := mk(fmt.Sprintf("gxfb-%d-abc", dead.Process.Pid))
	live := mk(fmt.Sprintf("gxfb-%d-def", os.Getpid()))
	other := mk("unrelated-1-x")
	legacy := mk("gxfb123456")

	sweepOrphans(root)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan of a dead pid was kept: %v", err)
	}
	for _, p := range []string{live, other, legacy} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", filepath.Base(p), err)
		}
	}
}

func TestMakeTempDir_NamesTheDirAfterThisPid(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir, err := makeTempDir()
	if err != nil {
		t.Fatal(err)
	}
	if pid, ok := ownerPid(filepath.Base(dir)); !ok || pid != os.Getpid() {
		t.Errorf("dir %q does not carry pid %d", dir, os.Getpid())
	}
}
