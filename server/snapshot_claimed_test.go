package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestSnapshot_ClaimedTicketCarriesLaunchTime(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	if err := ralphloop.Claim(path); err != nil {
		t.Fatal(err)
	}
	launched := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	h.Server.Registry().PutRunAt("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty", Session: agentrunner.Session{ID: "p1"}}, launched)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var claimed wire.TicketInfo
	servertest.WaitFor(t, 5*time.Second, "the ticket to show as claimed", func() bool {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range snap.Tickets {
			if tk.Address == "proj:epic-a/01" && tk.Status == "claimed" {
				claimed = tk
				return true
			}
		}
		return false
	})
	if !claimed.ClaimedAt.Equal(launched) {
		t.Fatalf("ClaimedAt = %v, want %v", claimed.ClaimedAt, launched)
	}
}
