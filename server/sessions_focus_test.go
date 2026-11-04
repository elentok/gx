package server_test

import (
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
)

func TestFocusSession_TargetsLastFocusedTUI(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSession(t)
	openTUI(ctx, t, h, "tui-1")
	openTUI(ctx, t, h, "tui-2")
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true})
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-2", Focused: true})
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-2"})

	res, err := h.Client.FocusSession(ctx, id)
	if err != nil || res.Refused || res.Client != "tui-2" {
		t.Fatalf("focus = %+v, %v; want target tui-2", res, err)
	}
	if ev := servertest.WaitForEvent(ctx, t, evs, wire.EventSessionFocus, ""); ev.Address != id || ev.Client != "tui-2" {
		t.Errorf("focus event = %+v, want session %s on tui-2", ev, id)
	}
}

func TestFocusSession_NoTUISucceedsWithNoTarget(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, _ := startFakeSession(t)
	drop := openTUI(ctx, t, h, "tui-1")
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true})
	drop()
	waitFor(t, func() bool {
		res, err := h.Client.ReportFocus(ctx, wire.FocusReport{Client: "tui-1"})
		return err == nil && res.Reason == wire.ReasonFocusNotConnected
	})

	res, err := h.Client.FocusSession(ctx, id)
	if err != nil || res.Refused || res.Client != "" {
		t.Fatalf("focus = %+v, %v; want success with no target", res, err)
	}
}

func TestFocusSession_RefusesUnknownID(t *testing.T) {
	t.Parallel()
	h, ctx, _, _, _ := startFakeSession(t)
	res, err := h.Client.FocusSession(ctx, "nope")
	if err != nil || res.Reason != wire.ReasonSessionUnknown {
		t.Fatalf("focus = %+v, %v; want %s", res, err, wire.ReasonSessionUnknown)
	}
}
