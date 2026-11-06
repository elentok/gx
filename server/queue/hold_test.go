package queue_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Queueing a held epic takes its hold off; queueing an unheld epic changes no hold.
func TestQueueAdd_ClearsTheHoldOfAHeldEpic(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "other", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rigs.WriteEpicFiles(t, store, "map-impl", "---\nstatus: open\n---\n", map[string]string{"01-a.md": rigs.IssueMD("01", "open", "", "A")})
	f := servertest.StartDesignsIn(t, store)

	for _, addr := range []string{"proj:plain", "proj:other", "proj:map"} {
		if res, err := f.H.Client.Admin.HoldEpic(f.Ctx, addr, true); err != nil || res.Refused {
			t.Fatalf("hold %s = %+v, %v", addr, res, err)
		}
	}
	held := func() []string {
		snap, err := f.H.Client.Snapshot(f.Ctx)
		if err != nil {
			t.Fatal(err)
		}
		return snap.Held
	}

	if res, err := f.H.Client.Queue.Add(f.Ctx, "proj:plain", ""); err != nil || res.Refused {
		t.Fatalf("queue add plain = %+v, %v", res, err)
	}
	if got := held(); slices.Contains(got, "proj:plain") || !slices.Contains(got, "proj:other") || !slices.Contains(got, "proj:map") {
		t.Fatalf("held after queueing proj:plain = %v, want only proj:plain released", got)
	}

	if res, err := f.H.Client.Queue.Add(f.Ctx, "proj:map-impl", ""); err != nil || res.Refused {
		t.Fatalf("queue add map-impl = %+v, %v", res, err)
	}
	if got := held(); slices.Contains(got, "proj:map") || !slices.Contains(got, "proj:other") {
		t.Fatalf("held after queueing proj:map-impl = %v, want its Design released", got)
	}
}
