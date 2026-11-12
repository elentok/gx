package merge

import (
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/testutil"
)

func TestEnsureFeatureWorktree_ConcurrentCallersAllSucceedAndShareOneWorktree(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	if out, err := exec.Command("git", "-C", repo, "branch", "epic-a").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}
	store := t.TempDir()
	testutil.WriteFile(t, store+"/proj", config.ProjectFileName, `{"repo": "`+repo+`"}`)
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{TicketStore: store} }, nil, nil, nil, nil, nil)
	m := &Merger{projects: reg}

	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() { errs <- m.ensureFeatureWorktree("proj", "epic-a", repo) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("ensureFeatureWorktree: %v", err)
		}
	}
	if _, err := os.Stat(reg.WorktreeDir("proj") + "/epic-a"); err != nil {
		t.Errorf("worktree missing: %v", err)
	}
}
