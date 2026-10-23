package server_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan server.Event
	repo string
	base string
	ck   *http.Cookie
}

// startDesigns starts a server with a map epic "map" and a plain epic "plain"
// in project proj, whose repo is the trunk checkout, with the web mux up.
func startDesigns(t *testing.T) designsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := testutil.TempRepo(t)
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	return designsFixture{h: h, ctx: ctx, evs: evs, repo: repo, base: hs.WebURL, ck: login(t, h.StateDir, hs.WebURL)}
}

func (f designsFixture) open(t *testing.T, epic string, isNew bool) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: epic, New: isNew})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f designsFixture) live(t *testing.T) []server.SessionInfo {
	t.Helper()
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []server.SessionInfo
	for _, s := range snap.Sessions {
		if s.Status.State != server.SessionExited {
			out = append(out, s)
		}
	}
	return out
}

func TestDesignsOpen_CreatesWithTheTuiPrefill(t *testing.T) {
	f := startDesigns(t)
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	meta := readMeta(t, f.h, res.Session.ID)
	want := map[string]any{"dir": f.repo, "name": "map", "ticket": "proj:map", "harness": string(agentfake.HarnessKind), "first_prompt": "/gx-design proj:map", "design": true}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
}

func TestDesignsOpen_ReturnsTheNewestLiveSession(t *testing.T) {
	f := startDesigns(t)
	mk := func() string {
		res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		return res.Session.ID
	}
	mk()
	newest := mk()
	if res := f.open(t, "proj:map", false); res.Session == nil || res.Session.ID != newest {
		t.Errorf("open = %+v, want session %s", res, newest)
	}
}

func TestDesignsOpen_ConcurrentCallsMakeOneSession(t *testing.T) {
	f := startDesigns(t)
	ids := make([]string, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: "proj:map"}); err == nil && res.Session != nil {
				ids[i] = res.Session.ID
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("ids = %v, want one shared session", ids)
		}
	}
	if n := len(f.live(t)); n != 1 {
		t.Errorf("%d live sessions, want 1", n)
	}
}

func TestDesignsOpen_NewStopsTheOldAndCreatesAnother(t *testing.T) {
	f := startDesigns(t)
	old := f.open(t, "proj:map", false).Session
	waitForHolder(t, filepath.Join(f.h.SessionsDir, old.ID))
	res := f.open(t, "proj:map", true)
	if res.Refused || res.Session == nil || res.Session.ID == old.ID {
		t.Fatalf("new = %+v, old %s", res, old.ID)
	}
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == old.ID && s.Status.State != server.SessionExited {
			t.Errorf("old session is %s, want Exited", s.Status.State)
		}
	}
}

func TestDesignsOpen_TicketAddressPromptsOnCreateAndSendsNothingToALiveSession(t *testing.T) {
	f := startDesigns(t)
	res := f.open(t, "proj:map/01", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if p := readMeta(t, f.h, res.Session.ID)["first_prompt"]; p != "/gx-design proj:map/01" {
		t.Errorf("first prompt = %v", p)
	}
	again := f.open(t, "proj:map/02", false)
	if again.Session == nil || again.Session.ID != res.Session.ID {
		t.Errorf("open on a live Design = %+v, want session %s", again, res.Session.ID)
	}
	if p := readMeta(t, f.h, res.Session.ID)["first_prompt"]; p != "/gx-design proj:map/01" {
		t.Errorf("first prompt changed to %v", p)
	}
}

func TestDesignsOpen_Refusals(t *testing.T) {
	f := startDesigns(t)
	for epic, want := range map[string]string{
		"proj:plain":   server.ReasonNotMapEpic,
		"proj:nope":    server.ReasonUnknownEpic,
		"nope:map":     server.ReasonUnknownEpic,
		"map":          server.ReasonSessionBadRequest,
		"proj:map/abc": server.ReasonSessionBadRequest,
	} {
		if res := f.open(t, epic, false); !res.Refused || res.Reason != want {
			t.Errorf("open %s = %+v, want %s", epic, res, want)
		}
	}
}

func TestWebDesigns_NewNeedsAFreshToken(t *testing.T) {
	f := startDesigns(t)
	live := f.open(t, "proj:map", false).Session
	waitForHolder(t, filepath.Join(f.h.SessionsDir, live.ID))
	post := func(body string) (int, string) {
		return webPostJSON(t, f.base+"/v1/designs/open", body, f.ck, "", jsonPost(f.base))
	}

	for name, body := range map[string]string{
		"no token":  `{"epic":"proj:map","new":true}`,
		"old token": fmt.Sprintf(`{"epic":"proj:map","new":true,"state":%q}`, live.ID+":starting:1"),
	} {
		if code, out := post(body); code != http.StatusOK || reasonOf(t, out) != server.ReasonStale {
			t.Errorf("new with %s = %d %s, want stale", name, code, out)
		}
	}
	if code, out := post(`{"epic":"proj:map"}`); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("plain call = %d %s, want it to need no token", code, out)
	}
	fresh := fmt.Sprintf(`{"epic":"proj:map","new":true,"state":%q}`, sessionToken(f.ctx, t, f.h, live.ID))
	if code, out := post(fresh); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("new with a fresh token = %d %s, want it to succeed", code, out)
	}
}
