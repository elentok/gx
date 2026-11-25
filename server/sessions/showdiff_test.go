package sessions_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// commonDirCounter wraps the real lookup and counts calls per directory.
type commonDirCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *commonDirCounter) option() func(*server.Config) {
	return func(cfg *server.Config) {
		cfg.DiffCommonDir = func(dir string) (string, error) {
			c.mu.Lock()
			c.calls[dir]++
			c.mu.Unlock()
			_, common, err := git.GitDirs(dir)
			return common, err
		}
	}
}

func (c *commonDirCounter) count(dir string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[dir]
}

func TestSessionInfo_ShowDiffDirFollowsTheProjectRepo(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	repo, linked := testutil.TempRepoWithLinkedWorktree(t, "feature")
	clone := testutil.TempRepo(t)
	plain := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "t", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	counter := &commonDirCounter{calls: map[string]int{}}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), counter.option())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cases := []struct {
		name, dir, fixOf string
		want             bool
	}{
		{"repo", repo, "", true},
		{"linked worktree", linked, "", true},
		{"separate clone", clone, "", false},
		{"non-repo dir", plain, "", false},
		{"fix agent in the repo", repo, "proj:epic/01", true},
		{"fix agent in a clone", clone, "proj:epic/02", false},
	}
	ids := map[string]string{}
	for _, c := range cases {
		res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{
			Project: "proj", Dir: c.dir, Name: c.name, FixOf: c.fixOf, Harness: string(agentfake.HarnessKind),
		})
		if err != nil || res.Session == nil {
			t.Fatalf("create %s: %+v, %v", c.name, res, err)
		}
		ids[c.name] = res.Session.ID
	}

	// Several list reads, then one git check per directory.
	var snap wire.Snapshot
	for range 3 {
		var err error
		if snap, err = h.Client.Snapshot(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		var got *wire.SessionInfo
		for i := range snap.Sessions {
			if snap.Sessions[i].ID == ids[c.name] {
				got = &snap.Sessions[i]
			}
		}
		if got == nil {
			t.Fatalf("%s: session %s missing from the snapshot", c.name, ids[c.name])
		}
		if got.ShowDiffDir != c.want {
			t.Errorf("%s: ShowDiffDir = %v, want %v", c.name, got.ShowDiffDir, c.want)
		}
	}
	for _, dir := range []string{repo, linked, clone, plain} {
		if n := counter.count(dir); n != 1 {
			t.Errorf("common-dir lookups for %s = %d, want 1", dir, n)
		}
	}
}
