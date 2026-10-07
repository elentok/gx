package server_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func queueAddresses(t *testing.T, h *servertest.Harness) []string {
	t.Helper()
	items, err := h.Client.QueueItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Address)
	}
	return got
}

func TestQueue_WritesChangeReadStreamAndSurviveRestart(t *testing.T) {
	store := t.TempDir()
	for _, id := range []string{"01", "02", "03"} {
		servertest.WriteTicket(t, store, "proj", "epic-a", id, "t"+id, "")
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	ctx := context.Background()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	for _, w := range []struct{ addr, agent string }{{"proj:epic-a/01", ""}, {"proj:epic-a/02", "codex"}, {"proj:epic-a/03", ""}} {
		if res, err := h.Client.QueueAdd(ctx, w.addr, w.agent); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", w.addr, res, err)
		}
	}
	if res, err := h.Client.QueueMove(ctx, "proj:epic-a/03", 1); err != nil || res.Refused {
		t.Fatalf("move: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueRemove(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("remove: %+v, %v", res, err)
	}

	want := []string{"proj:epic-a/03", "proj:epic-a/02"}
	if got := queueAddresses(t, h); !reflect.DeepEqual(got, want) {
		t.Errorf("queue = %v, want %v", got, want)
	}
	items, _ := h.Client.QueueItems(ctx)
	if items[0].Agent != "claude" || items[1].Agent != "codex" {
		t.Errorf("agents = %+v, want default claude then codex", items)
	}

	for i := 0; i < 5; i++ {
		select {
		case ev := <-evs:
			if ev.Type != server.EventQueueChanged {
				t.Errorf("event %d = %+v, want %s", i, ev, server.EventQueueChanged)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of 5 queue events streamed", i)
		}
	}

	h.Restart(t)
	if got := queueAddresses(t, h); !reflect.DeepEqual(got, want) {
		t.Errorf("queue after restart = %v, want %v", got, want)
	}
}

func TestQueue_Refusals(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicket(t, store, "proj", "map-epic", "01", "decide", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map-epic", "ticket.md"), []byte("---\nkind: map\nstatus: open\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	ctx := context.Background()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", ""); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		do     func() (server.QueueResult, error)
		reason string
	}{
		{"duplicate", func() (server.QueueResult, error) { return h.Client.QueueAdd(ctx, "proj:epic-a/01", "") }, server.ReasonAlreadyQueued},
		{"unknown ticket", func() (server.QueueResult, error) { return h.Client.QueueAdd(ctx, "proj:epic-a/99", "") }, server.ReasonUnknownTicket},
		{"bad address", func() (server.QueueResult, error) { return h.Client.QueueAdd(ctx, "nonsense", "") }, server.ReasonInvalidAddress},
		{"bad agent", func() (server.QueueResult, error) { return h.Client.QueueAdd(ctx, "proj:epic-a/02", "gpt") }, server.ReasonInvalidAgent},
		{"remove unqueued", func() (server.QueueResult, error) { return h.Client.QueueRemove(ctx, "proj:epic-a/02") }, server.ReasonNotQueued},
		{"move unqueued", func() (server.QueueResult, error) { return h.Client.QueueMove(ctx, "proj:epic-a/02", 1) }, server.ReasonNotQueued},
		{"map epic", func() (server.QueueResult, error) { return h.Client.QueueAdd(ctx, "proj:map-epic/01", "") }, server.ReasonMapEpic},
		{"move out of range", func() (server.QueueResult, error) { return h.Client.QueueMove(ctx, "proj:epic-a/01", 2) }, server.ReasonBadPosition},
	}
	for _, c := range cases {
		res, err := c.do()
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
	if got := queueAddresses(t, h); !reflect.DeepEqual(got, []string{"proj:epic-a/01"}) {
		t.Errorf("refusals changed the queue: %v", got)
	}
}

func TestQueue_RefusedWhileSchedulerIsInProcess(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "")
	if err != nil || !res.Refused || res.Reason != server.ReasonSchedulerNotSelected {
		t.Errorf("add = %+v, %v; want scheduler-not-selected", res, err)
	}
	if got := queueAddresses(t, h); len(got) != 0 {
		t.Errorf("queue = %v, want empty", got)
	}
}
