package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	// A conflicting merge exits 1; the test only needs the resulting state.
	_ = c.Run()
}

func TestBrief_ConflictsListsUnmergedFilesWithMarkerLines(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "base")
	gitIn(t, repo, "checkout", "-b", "side")
	testutil.WriteFile(t, repo, "a.txt", "one\nside\nthree\n")
	gitIn(t, repo, "commit", "-am", "side")
	gitIn(t, repo, "checkout", "main")
	testutil.WriteFile(t, repo, "a.txt", "one\nmain\nthree\n")
	gitIn(t, repo, "commit", "-am", "main")
	gitIn(t, repo, "merge", "side")

	out, code := runBriefCmd(t, repo, "conflicts")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	for _, want := range []string{"a.txt", "merge", "2:<<<<<<<", "4:=======", "6:>>>>>>>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBrief_ConflictsCleanTreeExitsZero(t *testing.T) {
	repo := testutil.TempRepo(t)
	out, code := runBriefCmd(t, repo, "conflicts")
	if code != 0 || !strings.Contains(out, "no conflicts") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}
