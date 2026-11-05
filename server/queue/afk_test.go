package queue_test

import (
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestAFK_ToggleSnapshotEventAndRestart(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := t.Context()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.SetAFK(ctx, nil)
	if err != nil || !res.AFK {
		t.Fatalf("toggle = %+v, %v; want on", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventAFKChanged, "")
	if snap, err := h.Client.Snapshot(ctx); err != nil || !snap.AFK {
		t.Fatalf("snapshot afk = %v, %v; want on", snap.AFK, err)
	}

	h.Restart(t)
	if snap, err := h.Client.Snapshot(ctx); err != nil || !snap.AFK {
		t.Fatalf("snapshot afk after restart = %v, %v; want on", snap.AFK, err)
	}
	off := false
	if res, err := h.Client.Sessions.SetAFK(ctx, &off); err != nil || res.AFK {
		t.Fatalf("set off = %+v, %v", res, err)
	}
	if res, err := h.Client.Sessions.SetAFK(ctx, &off); err != nil || res.AFK {
		t.Fatalf("set off again = %+v, %v; want still off", res, err)
	}
}
