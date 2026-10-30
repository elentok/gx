package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const repairToken = "0:repair"

// startFixAgent seeds epic "loop" in project proj with ticket 01 paused for repair.
func startFixAgent(t *testing.T, seed func(store string)) designsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
	servertest.WriteTicketWith(t, store, "proj", "loop", "02", "fine", servertest.TicketOpts{})
	if seed != nil {
		seed(store)
	}
	return startDesignsIn(t, store)
}

func (f designsFixture) fix(t *testing.T, address string) server.FixAgentResult {
	t.Helper()
	res, err := f.h.Client.FixAgent(f.ctx, server.FixAgentRequest{Address: address})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestFixAgent_StartsInTheTrunkCheckout(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, nil)
	res := f.fix(t, "proj:loop/01")
	if res.Refused || res.Session == nil {
		t.Fatalf("fix = %+v", res)
	}
	if res.RemoteControl {
		t.Error("remote_control is set before remote control exists")
	}
	meta := readMeta(t, f.h, res.Session.ID)
	want := map[string]any{"dir": f.repo, "name": "fix-proj_cloop_s01", "harness": string(agentfake.HarnessKind),
		"first_prompt": "/gx-investigate proj:loop/01 is paused for repair: Merge conflict"}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
	if v, ok := meta["ticket"]; ok && v != "" {
		t.Errorf("session ticket = %v, want none", v)
	}
	if _, ok := meta["worktree"]; ok && meta["worktree"] != "" {
		t.Errorf("session worktree = %v, want none", meta["worktree"])
	}
}

func TestFixAgent_AcceptsAnswerRepairAndErrorStates(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, func(store string) {
		servertest.WriteTicketWith(t, store, "proj", "loop", "03", "asking", servertest.TicketOpts{PauseReason: "manual"})
		servertest.WriteTicketWith(t, store, "proj", "loop", "04", "dangling", servertest.TicketOpts{BlockedBy: []string{"99"}})
	})
	for _, tc := range []struct{ addr, prompt string }{
		{"proj:loop/01", "/gx-investigate proj:loop/01 is paused for repair: Merge conflict"},
		{"proj:loop/03", "/gx-investigate proj:loop/03 is paused for an answer: "},
		{"proj:loop/04", "/gx-investigate proj:loop/04 is in error"},
	} {
		res := f.fix(t, tc.addr)
		if res.Refused || res.Session == nil {
			t.Fatalf("fix %s = %+v", tc.addr, res)
		}
		if got := readMeta(t, f.h, res.Session.ID)["first_prompt"].(string); !strings.HasPrefix(got, tc.prompt) {
			t.Errorf("fix %s prompt = %q, want prefix %q", tc.addr, got, tc.prompt)
		}
	}
}

func TestFixAgent_WebRefusesAnAnswerPausedTicket(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, func(store string) {
		servertest.WriteTicketWith(t, store, "proj", "loop", "03", "asking", servertest.TicketOpts{PauseReason: "manual"})
	})
	body, _ := json.Marshal(server.FixAgentRequest{Address: "proj:loop/03", State: "0:answer"})
	code, out := webPostJSON(t, f.base+"/v1/tickets/fix-agent", string(body), f.ck, "", jsonPost(f.base))
	if code != http.StatusOK || reasonOf(t, out) != server.ReasonFixNotRepair {
		t.Errorf("web fix = %d %s, want %s", code, out, server.ReasonFixNotRepair)
	}
}

func TestFixAgent_SurvivesResumingTheTicket(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, func(store string) {
		servertest.WriteTicketWith(t, store, "proj", "loop", "03", "asking", servertest.TicketOpts{PauseReason: "manual"})
	})
	if res := f.fix(t, "proj:loop/03"); res.Refused {
		t.Fatalf("fix = %+v", res)
	}
	res, err := f.h.Client.Repair(f.ctx, "resume", server.RepairRequest{Address: "proj:loop/03"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused {
		t.Fatalf("resume = %+v", res)
	}
	if n := len(f.live(t)); n != 1 {
		t.Errorf("%d live sessions after resume, want the Fix agent", n)
	}
}

func TestFixAgent_Refusals(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, func(store string) {
		servertest.WriteTicketWith(t, store, "proj", "loop", "03", "paused", servertest.TicketOpts{PauseReason: "manual"})
		servertest.WriteTicketWith(t, store, "proj", "loop", "04", "probe", servertest.TicketOpts{Type: "investigate", Parent: "03"})
	})
	for _, tc := range []struct{ addr, reason, msg string }{
		{"proj:loop/02", server.ReasonFixNotRepair, "not paused or in error"},
		{"proj:loop/03", server.ReasonFixInvestigating, "proj:loop/04"},
		{"proj:loop/99", server.ReasonFixUnknownTicket, "no ticket"},
	} {
		res := f.fix(t, tc.addr)
		if !res.Refused || res.Reason != tc.reason || !strings.Contains(res.Message, tc.msg) {
			t.Errorf("fix %s = %+v, want %s containing %q", tc.addr, res, tc.reason, tc.msg)
		}
	}
	if n := len(f.live(t)); n != 0 {
		t.Errorf("%d live sessions after refusals", n)
	}
}

func TestFixAgent_NoTrunkCheckout(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", t.TempDir()) // not a git repo, so no trunk
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)))
	res, err := h.Client.FixAgent(context.Background(), server.FixAgentRequest{Address: "proj:loop/01"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Refused || !strings.Contains(res.Message, "no trunk checkout") {
		t.Errorf("fix = %+v", res)
	}
}

func TestFixAgent_DuplicateStartIsRefused(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, nil)
	if res := f.fix(t, "proj:loop/01"); res.Refused {
		t.Fatalf("first fix = %+v", res)
	}
	res := f.fix(t, "proj:loop/01")
	if !res.Refused || res.Message != "an agent is already fixing proj:loop/01" {
		t.Errorf("second fix = %+v", res)
	}
}

func TestFixAgent_ConcurrentStartsCreateOneSession(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, nil)
	var wg sync.WaitGroup
	results := make([]server.FixAgentResult, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = f.fix(t, "proj:loop/01")
		}()
	}
	wg.Wait()
	started := 0
	for _, r := range results {
		if !r.Refused {
			started++
		}
	}
	if started != 1 || len(f.live(t)) != 1 {
		t.Errorf("started %d, live %d, want 1 and 1", started, len(f.live(t)))
	}
}

func TestFixAgent_WebNeedsTheStateToken(t *testing.T) {
	t.Parallel()
	f := startFixAgent(t, nil)
	post := func(state string) (int, string) {
		body, _ := json.Marshal(server.FixAgentRequest{Address: "proj:loop/01", State: state})
		return webPostJSON(t, f.base+"/v1/tickets/fix-agent", string(body), f.ck, "", jsonPost(f.base))
	}
	for _, state := range []string{"", "5:repair", "0:answer"} {
		if code, out := post(state); code != http.StatusOK || reasonOf(t, out) != server.ReasonStale {
			t.Errorf("state %q = %d %s, want stale", state, code, out)
		}
	}
	if n := len(f.live(t)); n != 0 {
		t.Fatalf("%d live sessions after stale tokens", n)
	}
	if code, out := post(repairToken); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("current token = %d %s", code, out)
	}
}
