package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func enqueuedAt(t *testing.T, items []server.QueueItem, addr string) time.Time {
	t.Helper()
	for _, it := range items {
		if it.Address == addr {
			return it.EnqueuedAt
		}
	}
	t.Fatalf("%s not in queue %+v", addr, items)
	return time.Time{}
}

func TestQueue_EnqueuedAt_AddRemoveMoveReplace(t *testing.T) {
	t.Parallel()
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
	items, err := h.Client.QueueItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.EnqueuedAt.IsZero() {
			t.Fatalf("%s has no enqueue time", it.Address)
		}
	}
	first := enqueuedAt(t, items, "proj:epic-a/01")

	res, err := h.Client.QueueMove(ctx, "proj:epic-a/01", 3)
	if err != nil || res.Refused {
		t.Fatalf("move: %+v, %v", res, err)
	}
	if got := enqueuedAt(t, res.Queue, "proj:epic-a/01"); !got.Equal(first) {
		t.Fatalf("move changed the time: %v -> %v", first, got)
	}

	// A client-sent time is ignored; a kept address keeps its server time.
	sent := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err = h.Client.QueueReplace(ctx, "proj", []server.QueueItem{
		{Address: "proj:epic-a/01", EnqueuedAt: sent},
		{Address: "proj:epic-a/02", EnqueuedAt: sent},
	})
	if err != nil || res.Refused {
		t.Fatalf("replace: %+v, %v", res, err)
	}
	if got := enqueuedAt(t, res.Queue, "proj:epic-a/01"); !got.Equal(first) {
		t.Fatalf("replace changed a kept time: %v -> %v", first, got)
	}
	if got := enqueuedAt(t, res.Queue, "proj:epic-a/02"); got.IsZero() || got.Equal(sent) {
		t.Fatalf("replace took the client time for a new address: %v", got)
	}

	if res, err := h.Client.QueueRemove(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("remove: %+v, %v", res, err)
	}
	res, err = h.Client.QueueAdd(ctx, "proj:epic-a/01", "")
	if err != nil || res.Refused {
		t.Fatalf("re-add: %+v, %v", res, err)
	}
	if got := enqueuedAt(t, res.Queue, "proj:epic-a/01"); !got.After(first) {
		t.Fatalf("re-add kept the old time: %v vs %v", got, first)
	}
}

func TestQueue_OldStateFileLoadsWithZeroTime(t *testing.T) {
	t.Parallel()
	h := startReplaceHarness(t)
	old := `[{"address":"proj:epic-a/01","agent":"claude"}]`
	if err := os.WriteFile(filepath.Join(h.StateDir, "queue.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	items, err := h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].EnqueuedAt.IsZero() {
		t.Fatalf("items = %+v, want one item with a zero time", items)
	}
}
