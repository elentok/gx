package tickets

import (
	"context"
	"slices"
	"testing"

	"github.com/elentok/gx/server"
)

func TestQueueEpic_QueuesOnlyOpenTicketsOfTheEpicInOrder(t *testing.T) {
	snap := server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:epic-impl/02", Status: "open"},
		{Address: "gx:epic-impl/01", Status: "open"},
		{Address: "gx:epic-impl/00", Status: "done"},
		{Address: "gx:other/01", Status: "open"},
		{Address: "x:epic-impl/01", Status: "open"},
	}}
	var got []string
	add := func(_ context.Context, addr, _ string) (server.QueueResult, error) {
		got = append(got, addr)
		return server.QueueResult{}, nil
	}
	queued, refusals := QueueEpic(context.Background(), snap, add, "gx:epic-impl")
	if want := []string{"gx:epic-impl/01", "gx:epic-impl/02"}; !slices.Equal(got, want) || queued != 2 || len(refusals) != 0 {
		t.Fatalf("queued %d %v refusals %v, want %v", queued, got, refusals, want)
	}
}
