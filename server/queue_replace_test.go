package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func addresses(items []server.QueueItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Address
	}
	return out
}

func startReplaceHarness(t *testing.T) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.WriteTicket(t, store, "other", "epic-b", "01", "solo", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	child := "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\n\n# child\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-a", "issues", "02-child.md"), []byte(child), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = 50 * time.Millisecond
	})
	registerLaunch(h)
	return h
}

func TestQueueReplace_SwapsOnlyOneProjectsEntries(t *testing.T) {
	h := startReplaceHarness(t)
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"proj:epic-a/01", "other:epic-b/01", "proj:epic-a/02"} {
		if res, err := h.Client.QueueAdd(ctx, a, ""); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", a, res, err)
		}
	}

	res, err := h.Client.QueueReplace(ctx, "proj", []server.QueueItem{{Address: "proj:epic-a/02", Agent: "codex"}})
	if err != nil || res.Refused {
		t.Fatalf("replace: %+v, %v", res, err)
	}
	got := addresses(res.Queue)
	if len(got) != 2 || got[0] != "other:epic-b/01" || got[1] != "proj:epic-a/02" || res.Queue[1].Agent != "codex" {
		t.Fatalf("queue = %+v, want other kept and proj swapped", res.Queue)
	}

	res, err = h.Client.QueueReplace(ctx, "proj", []server.QueueItem{{Address: "other:epic-b/01"}})
	if err != nil || !res.Refused || res.Reason != server.ReasonInvalidAddress {
		t.Fatalf("replace with a foreign ticket = %+v, %v", res, err)
	}
}

func TestQueueRemove_CascadesToDescendants(t *testing.T) {
	h := startReplaceHarness(t)
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"proj:epic-a/01", "other:epic-b/01", "proj:epic-a/02"} {
		if res, err := h.Client.QueueAdd(ctx, a, ""); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", a, res, err)
		}
	}
	res, err := h.Client.QueueRemove(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("remove: %+v, %v", res, err)
	}
	if got := addresses(res.Queue); len(got) != 1 || got[0] != "other:epic-b/01" {
		t.Fatalf("queue = %v, want only other:epic-b/01", got)
	}
}

func TestQueue_ReplaceAndRemoveRefuseWhileAnIterationIsLive(t *testing.T) {
	h := startReplaceHarness(t)
	ctx := context.Background()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	expectRun(t, h)

	res, err := h.Client.QueueRemove(ctx, "proj:epic-a/01")
	if err != nil || !res.Refused || res.Reason != server.ReasonTicketLive {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	res, err = h.Client.QueueReplace(ctx, "proj", nil)
	if err != nil || !res.Refused || res.Reason != server.ReasonTicketLive {
		t.Fatalf("replace = %+v, %v", res, err)
	}
	if got := addresses(mustItems(t, h)); len(got) != 1 {
		t.Fatalf("queue = %v, want it untouched", got)
	}
}

func mustItems(t *testing.T, h *servertest.Harness) []server.QueueItem {
	t.Helper()
	items, err := h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return items
}
