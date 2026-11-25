package epics_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestDetail_ShowDiffHasWorktree(t *testing.T) {
	t.Parallel()
	h, _ := viewDiffRig(t, "", func(store, repo string) {
		for id, opts := range map[string]servertest.TicketOpts{
			"01": {Status: "claimed"},
			"02": {PauseReason: "manual"},
			"03": {Status: "claimed"},
			"04": {},
			"05": {Status: "done"},
		} {
			servertest.WriteTicketWith(t, store, "proj", "epic", id, "t", opts)
		}
		for _, id := range []string{"01", "02", "04", "05"} {
			servertest.KeepWorktree(t, repo, "epic", id)
		}
	})
	for addr, want := range map[string]bool{
		"proj:epic/01": true,  // claimed, worktree on disk
		"proj:epic/02": true,  // paused, worktree on disk
		"proj:epic/03": false, // claimed, no worktree
		"proj:epic/04": false, // open, even with a worktree left over
		"proj:epic/05": false, // done, even with a worktree left over
	} {
		d, err := h.Client.Reads.Detail(context.Background(), addr)
		if err != nil {
			t.Fatal(err)
		}
		if d.HasWorktree != want {
			t.Errorf("%s: HasWorktree = %v, want %v", addr, d.HasWorktree, want)
		}
	}
}
