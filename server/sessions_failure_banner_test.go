package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// failureBannerFixture is a Design session on map epic proj:map whose server
// has banners on, a short stall threshold and poll interval, and (when web is
// set) the web listener; base is its URL.
type failureBannerFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan server.Event
	id   string
	conv string
	path string
	base string
}

func startFailureBannerSession(t *testing.T, web bool, opts ...func(*server.Config)) failureBannerFixture {
	t.Helper()
	cfg := t.TempDir()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all := []func(*server.Config){
		servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)),
		servertest.WithStallThreshold(time.Minute), servertest.WithChatPollInterval(20 * time.Millisecond), withAlerts(true, true),
		servertest.WithEnv("CLAUDE_CONFIG_DIR=" + cfg),
		func(c *server.Config) {
			if web {
				c.WebAddr = "127.0.0.1:0"
			}
		},
	}
	h := servertest.StartWithStore(t, store, append(all, opts...)...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map/01", Name: "fix-build"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, server.SessionWorking)
	return failureBannerFixture{h, ctx, evs, id, conv, filepath.Join(cfg, "projects", "-repo", conv+".jsonl"), hs.WebURL}
}

func (f failureBannerFixture) reach(t *testing.T, c agentfake.Control, state string) {
	t.Helper()
	reach(f.ctx, t, f.h, f.evs, f.id, f.conv, c, state)
}

// stall ages the transcript of the Working session past the threshold.
func (f failureBannerFixture) stall(t *testing.T) {
	t.Helper()
	writeTranscript(t, f.path, reply)
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(f.path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestFailureBanner_AuthSendsOneBanner(t *testing.T) {
	t.Parallel()
	f := startFailureBannerSession(t, false)
	writeTranscript(t, f.path, apiError401)
	f.reach(t, agentfake.Control{State: "done"}, server.SessionIdle)
	banners, _ := waitAlerts(f.h, 1)
	if len(banners) != 1 || banners[0].Body != "Claude login needed" {
		t.Fatalf("banners = %+v, want one for the login", banners)
	}
	if banners, _ = waitAlerts(f.h, 0); len(banners) != 1 {
		t.Errorf("banners = %+v, want none more while the failure stays", banners)
	}
}

func TestFailureBanner_StalledSendsOneBannerWhileWorking(t *testing.T) {
	t.Parallel()
	f := startFailureBannerSession(t, false)
	f.stall(t)
	banners, _ := waitAlerts(f.h, 1)
	if len(banners) != 1 || banners[0].Body != "No progress for 15 min" {
		t.Fatalf("banners = %+v, want one for the stall", banners)
	}
	if banners, _ = waitAlerts(f.h, 0); len(banners) != 1 {
		t.Errorf("banners = %+v, want none more while it stays stalled", banners)
	}
}

func TestFailureBanner_AFailureThatClearsAndReturnsSendsAgain(t *testing.T) {
	t.Parallel()
	f := startFailureBannerSession(t, false)
	f.stall(t)
	waitAlerts(f.h, 1)
	writeTranscript(t, f.path, reply, reply)
	waitForFailure(f.ctx, t, f.h, f.id, "")
	time.Sleep(100 * time.Millisecond) // negative wait: several polls, so one sees it clear
	f.stall(t)
	if banners, _ := waitAlerts(f.h, 2); len(banners) != 2 {
		t.Errorf("banners = %+v, want a second for the second stall", banners)
	}
}

func TestFailureBanner_CarriesTheDesignLinkOnlyWithTheWebListener(t *testing.T) {
	t.Parallel()
	for _, web := range []bool{true, false} {
		f := startFailureBannerSession(t, web)
		f.stall(t)
		banners, _ := waitAlerts(f.h, 1)
		if len(banners) != 1 {
			t.Fatalf("web %v: banners = %+v, want one", web, banners)
		}
		want := ""
		if web {
			want = f.base + "/#/d/proj:map"
		}
		if banners[0].Link != want || strings.Contains(banners[0].Link, "gx_token") {
			t.Errorf("web %v: link = %q, want %q", web, banners[0].Link, want)
		}
	}
}

func TestFailureBanner_ClickFocusesTheTUIOrCarriesTheLink(t *testing.T) {
	t.Parallel()
	f := startFailureBannerSession(t, true)
	openTUI(f.ctx, t, f.h, "tui-1")
	reportFocus(f.ctx, t, f.h, server.FocusReport{Client: "tui-1", Focused: true, TermProgram: "ghostty"})
	f.stall(t)
	banners, _ := waitAlerts(f.h, 1)
	if len(banners) != 1 || banners[0].Execute != "gx agents focus "+f.id || banners[0].Link == "" {
		t.Fatalf("banners = %+v, want the focus command with a TUI connected", banners)
	}

	g := startFailureBannerSession(t, true)
	g.stall(t)
	banners, _ = waitAlerts(g.h, 1)
	if len(banners) != 1 || banners[0].Execute != "" || banners[0].Link != g.base+"/#/d/proj:map" {
		t.Errorf("banners = %+v, want only the link with no TUI", banners)
	}
}

func TestFailureBanner_AFKChatCarriesTheLink(t *testing.T) {
	t.Parallel()
	chat := newSlackHook(t)
	f := startFailureBannerSession(t, true, func(c *server.Config) {
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	})
	started := chat.waitStarted(t)
	afk := true
	if _, err := f.h.Client.SetAFK(f.ctx, &afk); err != nil {
		t.Fatal(err)
	}
	f.stall(t)
	posts := chat.postsAfterAlert(f.h, started)
	if len(posts) != 1 || !strings.Contains(posts[0], f.base+"/#/d/proj:map") {
		t.Errorf("chat posts = %q, want one with the Design link", posts)
	}
}
