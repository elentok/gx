package cmdtest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestAddProject_ResolvesThroughRootForDirs(t *testing.T) {
	store := IsolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := AddProject(t, store, "mine", repo)

	got, err := tickets.RootForDirs(config.DirsFrom(EnvFor(t)), repo)
	if err != nil {
		t.Fatalf("RootForDirs: %v", err)
	}
	if got != project {
		t.Errorf("RootForDirs = %q, want %q", got, project)
	}
}

func TestMergeTestRepo_TwoWorktreesOnOneRepo(t *testing.T) {
	repoDir, mainDir := MergeTestRepo(t, map[string]string{"feature": "feature"})
	featureDir := filepath.Join(repoDir, "feature")

	out, err := exec.Command("git", "-C", repoDir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	var trees []string
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			trees = append(trees, p)
		}
	}
	for _, want := range []string{mainDir, featureDir} {
		found := false
		for _, tree := range trees {
			if r, _ := filepath.EvalSymlinks(tree); r == mustEval(t, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("worktree %q not listed in %v", want, trees)
		}
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
