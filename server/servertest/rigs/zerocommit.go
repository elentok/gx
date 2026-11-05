package rigs

import (
	"context"
	"github.com/elentok/gx/server/servertest"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

// RunToZeroCommitPause queues proj:epic-a/01 under an agent whose every turn
// ends with last as its final assistant text and no commit, so the loop pauses
// it zero-commit the way a real iteration does.
func RunToZeroCommitPause(t *testing.T, h *servertest.Harness, last string) {
	t.Helper()
	h.OnTurn(func(turn *servertest.Turn) { turn.Transcript(last) })
	QueueToZeroCommitPause(t, h)
}

// QueueToZeroCommitPause queues proj:epic-a/01 and waits for its pause.
func QueueToZeroCommitPause(t *testing.T, h *servertest.Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	// The loop's own finish debounce outlasts waitFor.
	servertest.WaitForEvent(ctx, t, evs, wire.EventIterationPaused, "proj:epic-a/01")
}
