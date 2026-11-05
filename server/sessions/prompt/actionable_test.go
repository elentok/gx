package prompt_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestActionable_DerivedFromBlocked(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)

	report(t, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	if want := (wire.SessionActionable{Kind: "permission", Reason: "run ls?"}); got.Actionable.Kind != want.Kind || got.Actionable.Reason != want.Reason {
		t.Errorf("blocked actionable = %+v, want %+v", got.Actionable, want)
	}
	if _, ok := readMeta(t, h, id)["actionable"]; ok {
		t.Error("a blocked-derived actionable was saved")
	}

	for state, c := range map[string]agentfake.Control{wire.SessionWorking: {State: "working"}, wire.SessionIdle: {State: "done"}} {
		report(t, conv, c)
		got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == state })
		if got.Actionable.Reason != "" {
			t.Errorf("%s: actionable = %+v, want none", state, got.Actionable)
		}
	}
}

func TestActionable_AgentRaisedClearsOnlyOnInput(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })

	action := []string{"/review", "/ship"}
	res, err := h.Client.MarkSessionActionable(ctx, wire.MarkActionableRequest{ID: id, Reason: "review ready", Action: action})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if a := res.Session.Actionable; a.Reason != "review ready" || !slices.Equal(a.Action, action) {
		t.Errorf("actionable = %+v, want the reason and action", a)
	}
	if saved, _ := readMeta(t, h, id)["actionable"].(map[string]any); saved["reason"] != "review ready" {
		t.Errorf("saved actionable = %v, want the reason", saved)
	}

	c, err := holder.DialHolder(filepath.Join(h.SessionsDir, id))
	if err != nil {
		t.Fatal(err)
	}
	att, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	if err := att.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if err := att.Scroll(1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Client.MarkSessionRead(ctx, id); err != nil {
		t.Fatal(err)
	}
	// A later status change orders the check after the holder saw the resize
	// and scroll.
	report(t, conv, agentfake.Control{State: "working"})
	if got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking }); got.Actionable.Reason == "" {
		t.Error("resize, scroll or viewing cleared the actionable")
	}

	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
	if _, ok := readMeta(t, h, id)["actionable"]; ok {
		t.Error("actionable still saved after input")
	}
}

func TestMarkActionable_Refusals(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	for name, tc := range map[string]struct {
		req    wire.MarkActionableRequest
		reason string
	}{
		"no reason":       {wire.MarkActionableRequest{ID: "abcdef"}, wire.ReasonSessionBadRequest},
		"unknown session": {wire.MarkActionableRequest{ID: "abcdef", Reason: "x"}, wire.ReasonSessionUnknown},
	} {
		res, err := h.Client.MarkSessionActionable(ctx, tc.req)
		if err != nil || !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: mark = %+v, %v; want refused %s", name, res, err, tc.reason)
		}
	}
}
