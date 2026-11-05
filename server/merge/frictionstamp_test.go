package merge_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// A ticket that paused once and then merged carries a friction summary with that pause.
// It stays in server: runMergeTicket needs the merge fixtures, which are root-only.
func TestFrictionStamp_MergedTicketCarriesItsPause(t *testing.T) {
	t.Parallel()
	f, tk := runMergeTicket(t, func(f *schedFixture, cwd string) string {
		servertest.AppendPauseRound(t, f.TicketFile(t, "01"), true)
		testutil.WriteFile(t, cwd, "fix.txt", "fix")
		testutil.CommitAll(t, cwd, "epic: Fix typecheck/lint after rebase onto main")
		return "finished"
	})
	if tk.Status != schema.StatusDone {
		t.Fatalf("status = %s, want done", tk.Status)
	}
	// The stamp lands just after the done event, so wait for it.
	servertest.Poll(5*time.Second, func() bool {
		var err error
		if tk, err = schema.ParseTicket(f.TicketFile(t, "01")); err != nil {
			t.Fatal(err)
		}
		return tk.Friction != nil
	})
	if tk.Friction == nil || tk.Friction.PausesByReason["zero-commit"] != 1 || tk.Friction.PauseEndings["cli"] != 1 {
		t.Errorf("friction = %+v, want one zero-commit pause ended by cli", tk.Friction)
	}
}
