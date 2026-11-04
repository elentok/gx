package queue_test

import (
	"github.com/elentok/gx/server/wire"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestViMode_SetClearToggleSnapshotEventAndRestart(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := t.Context()
	if snap, err := h.Client.Snapshot(ctx); err != nil || snap.ViMode {
		t.Fatalf("fresh snapshot vi_mode = %v, %v; want off", snap.ViMode, err)
	}
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.SetViMode(ctx, nil)
	if err != nil || !res.ViMode {
		t.Fatalf("toggle = %+v, %v; want on", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventViModeChanged, "")
	if snap, err := h.Client.Snapshot(ctx); err != nil || !snap.ViMode {
		t.Fatalf("snapshot vi_mode = %v, %v; want on", snap.ViMode, err)
	}

	h.Restart(t)
	if snap, err := h.Client.Snapshot(ctx); err != nil || !snap.ViMode {
		t.Fatalf("snapshot vi_mode after restart = %v, %v; want on", snap.ViMode, err)
	}
	off := false
	if res, err := h.Client.SetViMode(ctx, &off); err != nil || res.ViMode {
		t.Fatalf("set off = %+v, %v", res, err)
	}
	if res, err := h.Client.SetViMode(ctx, &off); err != nil || res.ViMode {
		t.Fatalf("set off again = %+v, %v; want still off", res, err)
	}
	on := true
	if res, err := h.Client.SetViMode(ctx, &on); err != nil || !res.ViMode {
		t.Fatalf("set on = %+v, %v", res, err)
	}
	if res, err := h.Client.SetViMode(ctx, nil); err != nil || res.ViMode {
		t.Fatalf("toggle from on = %+v, %v; want off", res, err)
	}
	h.Restart(t)
	if snap, err := h.Client.Snapshot(ctx); err != nil || snap.ViMode {
		t.Fatalf("snapshot vi_mode after off + restart = %v, %v; want off", snap.ViMode, err)
	}
}

func TestViMode_OneWritePublishesOneEvent(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := t.Context()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := h.Client.SetViMode(ctx, &on); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventViModeChanged, "")
	// A second write is a second event: the first wait consumed exactly one.
	if _, err := h.Client.SetViMode(ctx, nil); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventViModeChanged, "")
}
