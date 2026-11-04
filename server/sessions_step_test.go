package server_test

import (
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

func TestReportSessionStep_StoredWhileBusyAndClearedOnIdle(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })

	res, err := h.Client.ReportSessionStep(ctx, wire.ReportStepRequest{ID: id, Step: "resolve-tickets", Title: "Add the cache"})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("report step = %+v, %v", res, err)
	}
	if s := res.Session; s.ReportedStep != "resolve-tickets" || s.ReportedTitle != "Add the cache" {
		t.Errorf("session = step %q, title %q; want the reported ones", s.ReportedStep, s.ReportedTitle)
	}
	if saved := readMeta(t, h, id); saved["reported_step"] != "resolve-tickets" || saved["reported_title"] != "Add the cache" {
		t.Errorf("saved meta = %v, want the step and title", saved)
	}

	report(t, conv, agentfake.Control{State: "done"})
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if got.ReportedStep != "" || got.ReportedTitle != "" {
		t.Errorf("after idle = step %q, title %q; want both cleared", got.ReportedStep, got.ReportedTitle)
	}
}

func TestReportSessionStep_Refusals(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, _ := startFakeSession(t)
	for name, tc := range map[string]struct {
		req    wire.ReportStepRequest
		reason string
	}{
		"no step":         {wire.ReportStepRequest{ID: id}, wire.ReasonSessionBadRequest},
		"unknown session": {wire.ReportStepRequest{ID: "abcdef", Step: "spec"}, wire.ReasonSessionUnknown},
	} {
		res, err := h.Client.ReportSessionStep(ctx, tc.req)
		if err != nil || !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: report = %+v, %v; want refused %s", name, res, err, tc.reason)
		}
	}
}
