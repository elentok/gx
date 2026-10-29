package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestStop_WaitsForAMergeInFlightAndFlushesTheStoreCommit(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour // only the stop's flush can commit
	})
	h.OnTurn(func(turn *servertest.Turn) {
		testutil.WriteFile(t, turn.Cwd, "agent.txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
	})
	// The merge stops the iteration's agent; hold it there.
	releaseMerge := h.BlockStops()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for h.Stops() == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("merge never started")
		case <-time.After(10 * time.Millisecond):
		}
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
	h.OnTurn(func(turn *servertest.Turn) { turn.Stall() }) // the agent keeps working
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitFor(t, 20*time.Second, "a run to start", func() bool { return len(h.Server.Runs()) > 0 })
	if err := h.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if n := h.Stops(); n != 0 {
		t.Errorf("stop stopped %d live agents, want none", n)
	}
}
