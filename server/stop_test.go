package server_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestStop_WaitsForAMergeInFlightAndFlushesTheStoreCommit(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour // only the stop's flush can commit
	})
	_, _, cwd := registerLaunch(h)
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, _ []string) (any, herdrfake.Identities, error) {
		testutil.WriteFile(t, *cwd, "agent.txt", "work")
		testutil.CommitAll(t, *cwd, "agent work")
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle", "state_change_seq": 1}}, herdrfake.Identities{}, nil
	})
	// The merge closes the iteration's tab; hold it there.
	merging, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseMerge := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseMerge) // a failure before the release must not hold herdrfake forever
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		close(merging)
		<-release
		return map[string]any{}, herdrfake.Identities{}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	select {
	case <-merging:
	case <-ctx.Done():
		t.Fatal("merge never started")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- h.Stop() }()
	select {
	case err := <-stopped:
		t.Fatalf("stop returned during a merge: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	releaseMerge()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("stop: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("stop never returned after the merge finished")
	}

	data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil || !strings.Contains(string(data), "status: done") {
		t.Errorf("ticket not merged: %v\n%s", err, data)
	}
	if got := storeLog(t, store); !strings.Contains(got, "sync: ") {
		t.Errorf("store log = %q, want the stop to flush a commit", got)
	}
}

func TestStop_LeavesALiveAgentRunningAndUnmerged(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLaunch(h)
	closed := make(chan struct{}, 4)
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closed <- struct{}{}
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	// The agent keeps working: its finish wait, the first wait after the launch
	// prompt's wait for "working", blocks until the test ends. Counting waits
	// instead would also catch the start's own wait, which can repeat, and hang
	// Stop on the launch. A cleanup, not a defer, so the release merges before
	// the harness waits for the run to settle; the timeout turns a miss into a
	// failure, not a hang.
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	var prompted atomic.Bool
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if slices.Contains(argv, "working") {
			prompted.Store(true)
		} else if prompted.Load() {
			select {
			case <-block:
			case <-time.After(30 * time.Second):
			}
		}
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle", "state_change_seq": 1}}, herdrfake.Identities{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	for len(h.Server.Runs()) == 0 && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	if err := h.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-closed:
		t.Errorf("stop closed a live agent's tab")
	default:
	}
}
