package prflow_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
)

func createPR(t *testing.T, f *rigs.PRFlow) wire.QueueResult {
	t.Helper()
	res, err := f.H.Client.Tickets.EpicCreatePR(context.Background(), "proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func setProject(t *testing.T, f *rigs.PRFlow, extra string) {
	t.Helper()
	if err := servertest.WriteProjectRepo(f.Store, "proj", f.Origin.Local, extra); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePR_OpensAPRInPRAndMergeProjects(t *testing.T) {
	for name, extra := range map[string]string{"pr": `,"delivery":"pr"`, "merge": `,"delivery":"merge"`} {
		t.Run(name, func(t *testing.T) {
			f := openRig(t, goodText)
			setProject(t, f, extra)
			if res := createPR(t, f); res.Refused || res.State != string(prflow.OutcomeOpened) {
				t.Fatalf("result = %+v", res)
			}
			if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber == 0 || e.PRURL == "" {
				t.Errorf("epic not stamped: %d %q", e.PRNumber, e.PRURL)
			}
			if creates(f) != 1 {
				t.Errorf("pr create calls = %d", creates(f))
			}
		})
	}
}

func TestCreatePR_RefusalsEachHaveAReason(t *testing.T) {
	cases := map[string]struct {
		reason string
		setup  func(t *testing.T, f *rigs.PRFlow)
		addr   string
	}{
		"bad address": {reason: wire.ReasonInvalidAddress, addr: "epic-a"},
		"unknown":     {reason: wire.ReasonUnknownTicket, addr: "proj:nope"},
		"one-off": {reason: wire.ReasonOneOffsEpic, addr: "proj:" + tickets.OneOffsEpic, setup: func(t *testing.T, f *rigs.PRFlow) {
			servertest.WriteTicketWith(t, f.Store, "proj", tickets.OneOffsEpic, "01", "x", servertest.TicketOpts{Status: "done"})
		}},
		"incomplete": {reason: wire.ReasonNotComplete, setup: func(t *testing.T, f *rigs.PRFlow) {
			servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "more", servertest.TicketOpts{})
		}},
		"has a PR": {reason: wire.ReasonHasPR, setup: func(t *testing.T, f *rigs.PRFlow) { f.StampPR(t, 9, prURL) }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := openRig(t, goodText)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			addr := tc.addr
			if addr == "" {
				addr = "proj:epic-a"
			}
			res, err := f.H.Client.Tickets.EpicCreatePR(context.Background(), addr)
			if err != nil || !res.Refused || res.Reason != tc.reason {
				t.Fatalf("res = %+v, err %v; want refusal %s", res, err, tc.reason)
			}
			if creates(f) != 0 {
				t.Error("created a PR")
			}
		})
	}
}

func TestCreatePR_GhFailureIsTheResultNotAPause(t *testing.T) {
	f := openRig(t, goodText)
	f.GH.Update(func(s *ghfake.State) { s.LoggedOut = true })
	res := createPR(t, f)
	if !res.Refused || res.Reason != wire.ReasonPROpenFailed || res.Message == "" {
		t.Fatalf("res = %+v", res)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergeBlockKind != "" {
		t.Errorf("epic paused: %q", e.MergeBlockKind)
	}
}

func TestWeb_CreatePRChecksTheShownState(t *testing.T) {
	g := rigs.StartPRFlow(t, rigs.PRFlowOpts{Server: []func(*server.Config){func(c *server.Config) { c.WebAddr = "127.0.0.1:0" }}})
	servertest.WriteTicketWith(t, g.Store, "proj", "epic-a", "01", "root", servertest.TicketOpts{Status: "done"})
	servertest.CommitOnBranch(t, g.Origin.Local, "epic-a")
	hs, err := g.H.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("web_url = %q, err %v", hs.WebURL, err)
	}
	cookie := servertest.Login(t, g.H.StateDir, hs.WebURL)
	post := func(state string) (int, string) {
		body := `{"address":"proj:epic-a","state":"` + state + `"}`
		return servertest.WebPostJSON(t, hs.WebURL+"/v1/epics/create-pr", body, cookie, "", servertest.JSONPost(hs.WebURL))
	}
	if code, body := post(string(wire.EpicStateRunning)); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
		t.Errorf("stale = %d %s", code, body)
	}
	if creates(g) != 0 {
		t.Error("a stale card opened a PR")
	}
}

func autoRig(t *testing.T, extra string) *rigs.PRFlow {
	t.Helper()
	f := openRig(t, goodText)
	setProject(t, f, extra)
	f.Rescan(t)
	return f
}

const autoOn = `,"delivery":"pr","auto-pr":true`

func TestAutoPR_OpensOnTheNextTick(t *testing.T) {
	f := autoRig(t, autoOn)
	f.Tick(t)
	if creates(f) != 1 {
		t.Fatalf("pr create calls = %d", creates(f))
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber == 0 {
		t.Error("epic not stamped")
	}
}

func TestAutoPR_DoesNotOpen(t *testing.T) {
	cases := map[string]struct {
		extra string
		setup func(t *testing.T, f *rigs.PRFlow)
	}{
		"key off":       {extra: `,"delivery":"pr"`},
		"merge project": {extra: `,"delivery":"merge","auto-pr":true`},
		"after a close": {extra: autoOn, setup: func(t *testing.T, f *rigs.PRFlow) {
			for _, k := range []prflow.Kind{prflow.KindPROpened, prflow.KindPRClosed} {
				if err := prflow.Append(f.EpicDir(), prflow.Entry{Kind: k}); err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := autoRig(t, tc.extra)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			f.Tick(t)
			if creates(f) != 0 {
				t.Errorf("pr create calls = %d", creates(f))
			}
		})
	}
}

func TestAutoPR_OneOffNeverOpens(t *testing.T) {
	f := openRig(t, goodText)
	servertest.WriteTicketWith(t, f.Store, "proj", tickets.OneOffsEpic, "01", "x", servertest.TicketOpts{Status: "done"})
	// Only the one-offs epic is left to consider. Written before Auto-PR is on,
	// so the watcher's own tick never sees epic-a complete.
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "more", servertest.TicketOpts{})
	f.Rescan(t)
	setProject(t, f, autoOn)
	f.Rescan(t)
	f.Tick(t)
	if creates(f) != 0 {
		t.Errorf("pr create calls = %d", creates(f))
	}
}

func TestAutoPR_FlippingTheKeyAppliesOnTheNextTick(t *testing.T) {
	f := autoRig(t, `,"delivery":"pr"`)
	f.Tick(t)
	if creates(f) != 0 {
		t.Fatal("opened with the key off")
	}
	setProject(t, f, autoOn)
	f.Tick(t)
	if creates(f) != 1 {
		t.Errorf("pr create calls = %d after the key went on", creates(f))
	}
}
