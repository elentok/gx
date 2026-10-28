package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// A retro paused stuck and then cancelled still stamps its epic skipped.
func TestCancel_PausedRetroStampsItsEpicSkipped(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "work", "")
	epicTicket := filepath.Join(store, "proj", "epic-a", "ticket.md")
	if err := os.WriteFile(epicTicket, []byte("---\nstatus: open\nlabel: keep-me\n---\n\n# epic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oneOff := filepath.Join(store, "proj", "evolve-epic-a-20261010-1405")
	testutil.EnsureEpicTicketMD(t, oneOff)
	if err := os.MkdirAll(filepath.Join(oneOff, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	fm := "---\nid: \"01\"\nstatus: paused\npause_reason: merge-conflict\ntype: evolve\nunique: evolve:proj/epic-a\n---\n\n# retro\n"
	if err := os.WriteFile(filepath.Join(oneOff, "issues", "01-retro.md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:evolve-epic-a-20261010-1405/01", false); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	data, err := os.ReadFile(epicTicket)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"evolve_skipped: true", "evolved_at:", "label: keep-me"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("epic lacks %q:\n%s", want, data)
		}
	}
}
