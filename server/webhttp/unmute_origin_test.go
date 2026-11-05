package webhttp_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestUnmute_EndsTheRepairPausesRound(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	writeMuted(t, path, "open", "Body text.\n")
	if err := h.Server.PauseAs("proj:epic-a/01", events.MergeConflict, "stuck"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketUnmute(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("unmute = %+v, %v", res, err)
	}
	rounds := endedRounds(t, path)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowResume || rounds[0].End.Source != wire.OriginCLI {
		t.Errorf("rounds = %+v; want one ended by cli resume", rounds)
	}
}
