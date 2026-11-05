package rigs

import (
	"context"
	"github.com/elentok/gx/server/servertest"
	"testing"

	"github.com/elentok/gx/server/wire"
)

// EventStream opens the event stream from the current snapshot, so only later
// events arrive. It closes with the test.
func EventStream(t *testing.T, h *servertest.Harness) <-chan wire.Event {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}
