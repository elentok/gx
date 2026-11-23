package banners_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// waitPushes waits past the banner delay and returns the pushes sent; with want
// zero it waits the delay out first, since a push would arrive within it.
func waitPushes(h *servertest.Harness, want int) []servertest.Push {
	if want == 0 {
		waitAlerts(h, 0)
	}
	servertest.Poll(3*servertest.SessionBannerDelay, func() bool { return len(h.Pushes.Sent()) >= want })
	return h.Pushes.Sent()
}

func TestSessionPush_DoneBlockedActionableSendBannerText(t *testing.T) {
	t.Parallel()
	t.Run("done", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t)
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
		want := servertest.Push{Title: "proj · fix-build", Body: "Done", Path: "/agents/" + id, Tag: "session:" + id}
		if got := waitPushes(h, 1); len(got) != 1 || got[0] != want {
			t.Errorf("pushes = %+v, want [%+v]", got, want)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t)
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "Bash(rm -rf build)"}, wire.SessionBlocked)
		want := servertest.Push{Title: "proj · fix-build", Body: "Blocked: permission — Bash(rm -rf build)", Path: "/agents/" + id, Tag: "session:" + id, Urgent: true}
		if got := waitPushes(h, 1); len(got) != 1 || got[0] != want {
			t.Errorf("pushes = %+v, want [%+v]", got, want)
		}
	})
	t.Run("actionable", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t)
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
		res, err := h.Client.Sessions.MarkActionable(ctx, wire.MarkActionableRequest{ID: id, Reason: "review the plan"})
		if err != nil || res.Refused {
			t.Fatalf("mark actionable = %+v, %v", res, err)
		}
		want := servertest.Push{Title: "proj · fix-build", Body: "review the plan", Path: "/agents/" + id, Tag: "session:" + id, Urgent: true}
		if got := waitPushes(h, 1); len(got) != 1 || got[0] != want {
			t.Errorf("pushes = %+v, want [%+v]", got, want)
		}
	})
}

func TestSessionPush_MergesPerSessionTag(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startBannerSession(t)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
	res, err := h.Client.Sessions.MarkActionable(ctx, wire.MarkActionableRequest{ID: id, Reason: "review the plan"})
	if err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if got := waitPushes(h, 1); len(got) != 1 {
		t.Fatalf("pushes = %+v, want one for two triggers", got)
	}

	other, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Name: "other", Harness: string(agentfake.HarnessKind), Dir: t.TempDir()})
	if err != nil || other.Session == nil {
		t.Fatalf("create = %+v, %v", other, err)
	}
	conv2, _ := readMeta(t, h, other.Session.ID)["conversation_id"].(string)
	reach(ctx, t, h, evs, other.Session.ID, conv2, agentfake.Control{State: "working"}, wire.SessionWorking)
	reach(ctx, t, h, evs, other.Session.ID, conv2, agentfake.Control{State: "done"}, wire.SessionIdle)
	got := waitPushes(h, 2)
	if len(got) != 2 || got[0].Tag == got[1].Tag {
		t.Errorf("pushes = %+v, want two with different tags", got)
	}
}

func TestSessionPush_IndependentOfAFKAndBannerSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		banners, sounds bool
	}{{"banners off", false, true}, {"sounds off", true, false}, {"both off", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t, withAlerts(tc.banners, tc.sounds))
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
			if got := waitPushes(h, 1); len(got) != 1 {
				t.Errorf("pushes = %+v, want one", got)
			}
		})
	}
	t.Run("focused TUI with AFK on", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t)
		afk := true
		if _, err := h.Client.Sessions.SetAFK(ctx, &afk); err != nil {
			t.Fatal(err)
		}
		openTUI(ctx, t, h, "tui-1")
		reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true, Session: id})
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
		if got := waitPushes(h, 1); len(got) != 1 {
			t.Errorf("pushes = %+v, want one", got)
		}
		if banners := h.Banners.Sent(); len(banners) != 0 {
			t.Errorf("banners = %+v, want none while the TUI is focused", banners)
		}
	})
}

func TestSessionPush_SkippedOnlyWhenOpenOnScreen(t *testing.T) {
	t.Parallel()
	t.Run("focused TUI with AFK off", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t)
		openTUI(ctx, t, h, "tui-1")
		reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true, Session: id})
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
		if got := waitPushes(h, 0); len(got) != 0 {
			t.Errorf("pushes = %+v, want none", got)
		}
	})
	t.Run("visible web tab", func(t *testing.T) {
		h, ctx, evs, id, conv := startBannerSession(t, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
		hs, err := h.Client.Handshake(ctx)
		if err != nil || hs.WebURL == "" {
			t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
		}
		cookie := servertest.Login(t, h.StateDir, hs.WebURL)
		tabCtx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(tabCtx, http.MethodGet, hs.WebURL+"/v1/events?client=web-1", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(cookie)
		resp, err := servertest.WebClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		waitFor(t, func() bool {
			body := `{"client":"web-1","focused":true,"session":"` + id + `"}`
			code, res := servertest.WebPostJSON(t, hs.WebURL+"/v1/sessions/focus", body, cookie, "", servertest.JSONPost(hs.WebURL))
			return code == http.StatusOK && !strings.Contains(res, wire.ReasonFocusNotConnected)
		})
		reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
		if got := waitPushes(h, 0); len(got) != 0 {
			t.Errorf("pushes = %+v, want none", got)
		}
	})
}

func TestSessionPush_FailureOncePerKind(t *testing.T) {
	t.Parallel()
	f := startFailureBannerSession(t, false)
	writeTranscript(t, f.path, apiError401)
	f.reach(t, agentfake.Control{State: "done"}, wire.SessionIdle)
	got := waitPushes(f.h, 1)
	if len(got) != 1 || !got[0].Urgent || got[0].Body != "Claude login needed" {
		t.Fatalf("pushes = %+v, want one urgent for the login", got)
	}
	if got = waitPushes(f.h, 0); len(got) != 1 {
		t.Errorf("pushes = %+v, want none more while the failure stays", got)
	}
}

func TestSessionPush_DesignRoundGoesToDesignPage(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", store)
	writeAssistantMessage(t, path, "Round one\n\n❓ **Q1** Which store?")
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, wire.SessionWorking)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "idle"}, wire.SessionIdle)
	addr, err := tickets.ParseQueuedAddress("proj:map")
	if err != nil {
		t.Fatal(err)
	}
	got := waitPushes(h, 1)
	if len(got) != 1 || !got[0].Urgent || got[0].Path != "/d/"+queue.RootOf(addr).String() {
		t.Fatalf("pushes = %+v, want one urgent for /d/%s", got, queue.RootOf(addr))
	}
	if strings.Contains(got[0].Path, "://") {
		t.Errorf("path = %q, want a route path, not a URL", got[0].Path)
	}
}
