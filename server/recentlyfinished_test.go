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

// writeOneOff writes a one-off ticket in epic; stamp sets at as completed_at,
// otherwise at becomes the file's mod time.
func writeOneOff(t *testing.T, store, epic, id, status string, at time.Time, stamp bool) {
	t.Helper()
	servertest.WriteTicketWith(t, store, "proj", epic, id, "oo"+id, servertest.TicketOpts{Status: status})
	matches, _ := filepath.Glob(filepath.Join(store, "proj", epic, "issues", id+"-*.md"))
	if len(matches) != 1 {
		t.Fatalf("ticket file for %s/%s: %v", epic, id, matches)
	}
	if !stamp {
		if err := os.Chtimes(matches[0], at, at); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), "---\n", "---\ncompleted_at: "+at.Format(time.RFC3339)+"\n", 1))
	if err := os.WriteFile(matches[0], b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshot_RecentlyFinishedMixesOneOffsWithEpics(t *testing.T) {
	store := t.TempDir()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	writeFinishedEpic(t, store, "e1", base.Add(1*time.Hour))
	writeFinishedEpic(t, store, "e2", base.Add(4*time.Hour))
	writeOneOff(t, store, server.OneOffsArchivedEpic, "01", "done", base.Add(3*time.Hour), true)
	writeOneOff(t, store, server.OneOffsArchivedEpic, "02", "cancelled", base.Add(9*time.Hour), false)
	writeOneOff(t, store, server.OneOffsEpic, "03", "done", base.Add(2*time.Hour), false)
	writeOneOff(t, store, server.OneOffsEpic, "04", "open", base.Add(8*time.Hour), false)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.RecentlyFinished = 4 })

	got := finishedNames(t, h)
	want := []struct {
		addr   string
		oneOff bool
	}{
		{"proj:e2", false},
		{"proj:" + server.OneOffsArchivedEpic + "/01", true},
		{"proj:" + server.OneOffsEpic + "/03", true},
		{"proj:e1", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %d entries", got, len(want))
	}
	for i, w := range want {
		if got[i].Address != w.addr || got[i].OneOff != w.oneOff {
			t.Errorf("entry %d = %+v, want %s oneOff=%v", i, got[i], w.addr, w.oneOff)
		}
		if w.oneOff && (got[i].Done != 0 || got[i].Total != 0) {
			t.Errorf("one-off entry %d has counts: %+v", i, got[i])
		}
	}
}

func TestSnapshot_RecentlyFinishedZeroListsNone(t *testing.T) {
	store := t.TempDir()
	writeFinishedEpic(t, store, "e1", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.RecentlyFinished = 0 })
	if got := finishedNames(t, h); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
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
