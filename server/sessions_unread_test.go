package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestUnread_RaisedByDoneBlockedAndCrashNotCancel(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	step := func(c agentfake.Control, state string, want bool) {
		t.Helper()
		report(t, conv, c)
		got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == state })
		if got.Unread.Unread != want {
			t.Fatalf("%s %s: unread = %v, want %v", c.State, state, got.Unread.Unread, want)
		}
	}

	step(agentfake.Control{State: "working"}, wire.SessionWorking, false)
	step(agentfake.Control{State: "done"}, wire.SessionIdle, true)
	if saved, _ := readMeta(t, h, id)["unread"].(map[string]any); saved["unread"] != true {
		t.Errorf("saved unread = %v, want unread", saved)
	}
	markRead(ctx, t, h, id)
	if _, ok := readMeta(t, h, id)["unread"]; ok {
		t.Error("unread still saved after mark read")
	}

	step(agentfake.Control{State: "working"}, wire.SessionWorking, false)
	step(agentfake.Control{State: "idle"}, wire.SessionIdle, false)
	step(agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"}, wire.SessionBlocked, true)
	markRead(ctx, t, h, id)

	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit, Code: 1}); err != nil {
		t.Fatal(err)
	}
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if !got.Unread.Unread {
		t.Error("crash raised no unread")
	}
}

func TestUnread_FocusedTUIViewingSuppressesAndClears(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	tuiCtx, dropTUI := context.WithCancel(ctx)
	defer dropTUI()
	tuiEvs, err := h.Client.EventsAs(tuiCtx, 0, "tui-1")
	if err != nil {
		t.Fatal(err)
	}
	// Flushed, so the server never drops the stream for lagging.
	go func() {
		for range tuiEvs {
		}
	}()
	turn := func() wire.SessionInfo {
		t.Helper()
		report(t, conv, agentfake.Control{State: "working"})
		waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
		report(t, conv, agentfake.Control{State: "done"})
		return waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	}

	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true, Session: id, TermProgram: "ghostty"})
	if got := turn(); got.Unread.Unread {
		t.Error("done raised unread while on screen in a focused TUI")
	}
	report(t, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	if got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked }); got.Unread.Unread {
		t.Error("blocked raised unread while on screen in a focused TUI")
	}

	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Session: id})
	if got := turn(); !got.Unread.Unread {
		t.Error("a blurred TUI suppressed unread")
	}
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true, Session: id})
	if got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return !s.Unread.Unread }); got.Unread.Unread {
		t.Error("viewing did not clear unread")
	}

	dropTUI()
	servertest.WaitFor(t, 5*time.Second, "the focus report to be refused after its stream closed", func() bool {
		res, err := h.Client.ReportFocus(ctx, wire.FocusReport{Client: "tui-1", Focused: true})
		if err != nil {
			t.Fatal(err)
		}
		return res.Refused && res.Reason == wire.ReasonFocusNotConnected
	})
	if got := turn(); !got.Unread.Unread {
		t.Error("a dropped TUI's report still suppressed unread")
	}
}

func TestReportFocus_RefusesUnknownClientOrSession(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.EventsAs(ctx, 0, "tui-1"); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		req    wire.FocusReport
		reason string
	}{
		"no client":       {wire.FocusReport{Focused: true}, wire.ReasonSessionBadRequest},
		"no stream":       {wire.FocusReport{Client: "tui-2", Focused: true}, wire.ReasonFocusNotConnected},
		"unknown session": {wire.FocusReport{Client: "tui-1", Session: "abcdef"}, wire.ReasonSessionUnknown},
	} {
		res, err := h.Client.ReportFocus(ctx, tc.req)
		if err != nil || !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: report = %+v, %v; want refused %s", name, res, err, tc.reason)
		}
	}
	if res, err := h.Client.MarkSessionRead(ctx, "abcdef"); err != nil || res.Reason != wire.ReasonSessionUnknown {
		t.Errorf("mark read unknown = %+v, %v; want refused %s", res, err, wire.ReasonSessionUnknown)
	}
}

func TestMarkRead_FailedSaveIsAnError(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	report(t, conv, agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Unread.Unread })
	dir := filepath.Join(h.SessionsDir, id)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if res, err := h.Client.MarkSessionRead(ctx, id); err == nil {
		t.Errorf("mark read with an unsavable meta = %+v, nil; want an error", res)
	}
}

func markRead(ctx context.Context, t *testing.T, h *servertest.Harness, id string) {
	t.Helper()
	res, err := h.Client.MarkSessionRead(ctx, id)
	if err != nil || res.Refused || res.Session == nil || res.Session.Unread.Unread {
		t.Fatalf("mark read = %+v, %v; want the session read", res, err)
	}
}
