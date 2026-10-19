package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

// writeFinishedEpic writes epic with one done ticket, stamped completed at.
func writeFinishedEpic(t *testing.T, store, epic string, at time.Time) {
	t.Helper()
	servertest.WriteTicketWith(t, store, "proj", epic, "01", "t", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", epic, "02", "u", servertest.TicketOpts{})
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", epic), map[string]any{"completed_at": at}); err != nil {
		t.Fatal(err)
	}
}

func finishedNames(t *testing.T, h *servertest.Harness) []server.FinishedEpic {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return snap.RecentlyFinished
}

func TestSnapshot_RecentlyFinishedIsCappedOrderedAndSkipsMaps(t *testing.T) {
	store := t.TempDir()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i, name := range []string{"e1", "e2", "e3", "e4"} {
		writeFinishedEpic(t, store, name, base.Add(time.Duration(i)*time.Hour))
	}
	writeFinishedEpic(t, store, "map", base.Add(10*time.Hour))
	mapMD := filepath.Join(store, "proj", "map", "ticket.md")
	if err := os.WriteFile(mapMD, []byte("---\nkind: map\nstatus: open\ncompleted_at: "+base.Add(10*time.Hour).Format(time.RFC3339)+"\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.RecentlyFinished = 3 })

	got := finishedNames(t, h)
	var names []string
	for _, f := range got {
		names = append(names, f.Name)
	}
	if want := []string{"e4", "e3", "e2"}; len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if got[0].Project != "proj" || got[0].Done != 1 || got[0].Total != 2 || !got[0].CompletedAt.Equal(base.Add(3*time.Hour)) {
		t.Fatalf("entry = %+v", got[0])
	}

	h.Restart(t)
	again := finishedNames(t, h)
	if len(again) != 3 || again[0].Name != "e4" || again[2].Name != "e2" {
		t.Fatalf("after restart = %+v", again)
	}
}

func TestSnapshot_RecentlyFinishedDefaultsToFive(t *testing.T) {
	store := t.TempDir()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range 7 {
		writeFinishedEpic(t, store, "e"+string(rune('a'+i)), base.Add(time.Duration(i)*time.Hour))
	}
	h := servertest.StartWithStore(t, store)
	if got := finishedNames(t, h); len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
}
