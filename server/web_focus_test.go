package server_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// webFocus is a banner session served on a web mux with a recording chat hook.
type webFocus struct {
	h       *servertest.Harness
	base    string
	cookie  *http.Cookie
	chat    *slackHook
	started int
	id      string
	step    func(agentfake.Control, string)
}

func startWebFocus(t *testing.T) *webFocus {
	t.Helper()
	chat := newSlackHook(t)
	gate := filepath.Join(t.TempDir(), "gate.json")
	h, ctx, evs, id, conv := startBannerSession(t, func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: gate, FlushInterval: testFlushInterval}
	})
	started := chat.waitStarted(t)
	afk := true
	if _, err := h.Client.SetAFK(ctx, &afk); err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	return &webFocus{h: h, base: hs.WebURL, cookie: login(t, h.StateDir, hs.WebURL), chat: chat, started: started, id: id,
		step: func(c agentfake.Control, state string) { reach(ctx, t, h, evs, id, conv, c, state) }}
}

// openTab opens the web tab's event stream.
func (f *webFocus) openTab(t *testing.T, client string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.base+"/v1/events?client="+client, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.cookie)
	resp, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
}

func (f *webFocus) report(t *testing.T, client string, focused bool, session string) string {
	t.Helper()
	body := fmt.Sprintf(`{"client":%q,"focused":%t,"session":%q}`, client, focused, session)
	code, res := webPostJSON(t, f.base+"/v1/sessions/focus", body, f.cookie, "", jsonPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("web focus = %d %s", code, res)
	}
	return res
}

func (f *webFocus) reportOnceConnected(t *testing.T, client string, focused bool, session string) {
	t.Helper()
	waitFor(t, func() bool {
		return !strings.Contains(f.report(t, client, focused, session), server.ReasonFocusNotConnected)
	})
}

func TestWebFocus_VisibleTabSilencesBannerAndAFKChat(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	f.openTab(t, "web-1")
	f.reportOnceConnected(t, "web-1", true, f.id)
	f.step(agentfake.Control{State: "done"}, server.SessionIdle)
	if posts := f.chat.postsAfterAlert(f.h, f.started); len(posts) != 0 {
		t.Errorf("chat posts = %q, want none", posts)
	}
	if banners, sounds := f.h.Banners.Sent(), f.h.Sounds.Played(); len(banners) != 0 || len(sounds) != 0 {
		t.Errorf("banners = %+v, sounds = %v; want none", banners, sounds)
	}
}

func TestWebFocus_HiddenTabSilencesNothing(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	f.openTab(t, "web-1")
	f.reportOnceConnected(t, "web-1", false, f.id)
	f.step(agentfake.Control{State: "done"}, server.SessionIdle)
	if posts := f.chat.postsAfterAlert(f.h, f.started); len(posts) != 1 {
		t.Errorf("chat posts = %q, want one", posts)
	}
	if banners := f.h.Banners.Sent(); len(banners) != 1 {
		t.Errorf("banners = %+v, want one", banners)
	}
}

func TestWebFocus_TUIStillSilencesBannerOnly(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	ctx := t.Context()
	openTUI(ctx, t, f.h, "tui-1")
	reportFocus(ctx, t, f.h, server.FocusReport{Client: "tui-1", Focused: true, Session: f.id})
	f.step(agentfake.Control{State: "done"}, server.SessionIdle)
	if posts := f.chat.postsAfterAlert(f.h, f.started); len(posts) != 1 {
		t.Errorf("chat posts = %q, want one", posts)
	}
	if banners := f.h.Banners.Sent(); len(banners) != 0 {
		t.Errorf("banners = %+v, want none", banners)
	}
}

func TestWebFocus_ClickTargetStaysOnLastFocusedTUI(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	ctx := t.Context()
	openTUI(ctx, t, f.h, "tui-1")
	reportFocus(ctx, t, f.h, server.FocusReport{Client: "tui-1", Focused: true})
	f.openTab(t, "web-1")
	f.reportOnceConnected(t, "web-1", true, "")

	res, err := f.h.Client.FocusSession(ctx, f.id)
	if err != nil || res.Refused || res.Client != "tui-1" {
		t.Fatalf("focus = %+v, %v; want target tui-1", res, err)
	}
	f.step(agentfake.Control{State: "done"}, server.SessionIdle)
	banners, _ := waitAlerts(f.h, 1)
	if len(banners) != 1 || banners[0].Execute != "gx agents focus "+f.id {
		t.Errorf("banners = %+v, want click to focus the TUI", banners)
	}
}

func TestWebFocus_RefusedWithoutAnOpenStream(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	if res := f.report(t, "web-1", true, f.id); !strings.Contains(res, server.ReasonFocusNotConnected) {
		t.Errorf("result = %s, want %s", res, server.ReasonFocusNotConnected)
	}
}

func TestWebFocus_FocusedReportMarksSessionRead(t *testing.T) {
	t.Parallel()
	f := startWebFocus(t)
	ctx := t.Context()
	f.step(agentfake.Control{State: "done"}, server.SessionIdle)
	if !sessionUnread(ctx, t, f.h, f.id) {
		t.Fatal("session not unread after done")
	}
	f.openTab(t, "web-1")
	f.reportOnceConnected(t, "web-1", true, f.id)
	if sessionUnread(ctx, t, f.h, f.id) {
		t.Error("session still unread after a focused web report")
	}
}

func sessionUnread(ctx context.Context, t *testing.T, h *servertest.Harness, id string) bool {
	t.Helper()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return s.Unread.Unread
		}
	}
	t.Fatalf("no session %s", id)
	return false
}
