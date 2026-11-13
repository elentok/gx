package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// handoffFixture is a design session launched by gx on a map at the chart
// step, Idle on a fake program with an input box.
type handoffFixture struct {
	designsFixture
	store, cfg, id, conv string
}

func startHandoff(t *testing.T, step string, opts ...func(*server.Config)) handoffFixture {
	t.Helper()
	s := rigs.StartHandoff(t, step, opts...)
	d := s.DesignsFixture
	return handoffFixture{
		designsFixture: designsFixture{h: d.H, ctx: d.Ctx, evs: d.Evs, repo: d.Repo, base: d.Base, ck: d.Ck},
		store:          s.Store, cfg: s.Cfg, id: s.ID, conv: s.Conv,
	}
}

func (f handoffFixture) shared() rigs.HandoffFixture {
	return rigs.HandoffFixture{DesignsFixture: f.designsFixture.shared(), Store: f.store, Cfg: f.cfg, ID: f.id, Conv: f.conv}
}

func (f handoffFixture) continueSession(t *testing.T) {
	t.Helper()
	f.shared().ContinueSession(t)
}

func (f handoffFixture) newConversation(t *testing.T, old string) map[string]any {
	t.Helper()
	return f.shared().NewConversation(t, old)
}

func (f handoffFixture) wantRestartedOnWorker(t *testing.T) {
	t.Helper()
	f.shared().WantRestartedOnWorker(t)
}

func TestChartingHandoff_ContinueRestartsUnderWorkerArgs(t *testing.T) {
	t.Parallel()
	f := startHandoff(t, "chart")
	before := readMeta(t, f.h, f.id)
	if before["tier"] != "strongest" {
		t.Fatalf("tier = %v, want strongest", before["tier"])
	}
	setMapStep(t, f.store, "resolve-tickets")
	f.continueSession(t)
	f.wantRestartedOnWorker(t)
	if after := readMeta(t, f.h, f.id); after["name"] != before["name"] {
		t.Errorf("name = %v, want %v", after["name"], before["name"])
	}
}

func TestChartingHandoff_AutoAdvanceRestartsUnderWorkerArgs(t *testing.T) {
	t.Parallel()
	f := startHandoff(t, "chart", servertest.WithAutoAdvanceDelay(200*time.Millisecond))
	setMapStep(t, f.store, "resolve-tickets")
	writeAssistantMessage(t, filepath.Join(f.cfg, "projects", "-repo", f.conv+".jsonl"), runEndText)
	rigs.EndTurn(t, &rigs.GrillingFixture{H: f.h, Ctx: f.ctx, Evs: f.evs, ID: f.id, Conv: f.conv})
	f.wantRestartedOnWorker(t)
}

func TestChartingHandoff_FailedRestartExitsCrashedAndResumeWorks(t *testing.T) {
	t.Parallel()
	f := startHandoff(t, "chart")
	dir := readMeta(t, f.h, f.id)["dir"].(string)
	setMapStep(t, f.store, "resolve-tickets")
	setWorkerModel(t, f.store, "stale-model")
	// A session dir that is gone stands in for a stale worker slug: the new
	// agent cannot start.
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	f.continueSession(t)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionExited && s.Exit == wire.SessionExitCrashed
	})
	if err := os.Rename(moved, dir); err != nil {
		t.Fatal(err)
	}
	if model := readMeta(t, f.h, f.id)["model"]; model != "stale-model" {
		t.Fatalf("model after the failed restart = %v, want stale-model", model)
	}
	setWorkerModel(t, f.store, "sonnet")
	res, err := f.h.Client.Sessions.Resume(f.ctx, f.id)
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State != wire.SessionExited })
	if model := readMeta(t, f.h, f.id)["model"]; model != "sonnet" {
		t.Errorf("model after Resume = %v, want the fixed config's sonnet", model)
	}
}

func setWorkerModel(t *testing.T, store, model string) {
	t.Helper()
	body := `{"name":"proj","agents":{"claude":{"model":"` + model + `"}}}`
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChartingHandoff_NothingRestartsOutsideAStrongestDesignLaunch(t *testing.T) {
	t.Parallel()
	t.Run("worker session", func(t *testing.T) {
		f := startHandoff(t, "resolve-tickets")
		f.continueSession(t)
		wantResetInPlace(t, f, "worker")
	})
	t.Run("still charting", func(t *testing.T) {
		f := startHandoff(t, "chart")
		f.continueSession(t)
		wantResetInPlace(t, f, "strongest")
	})
	t.Run("linked later", func(t *testing.T) {
		f := startHandoff(t, "chart")
		setMapStep(t, f.store, "resolve-tickets")
		// A session linked to the design later has no launch kind.
		res, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		id := res.Session.ID
		conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
		waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
		report(t, conv, agentfake.Control{State: "idle"})
		waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
		other := handoffFixture{designsFixture: f.designsFixture, id: id, conv: conv}
		other.continueSession(t)
		wantResetInPlace(t, other, "worker")
	})
	t.Run("triage session", func(t *testing.T) {
		f := startHandoff(t, "chart")
		setMapStep(t, f.store, "resolve-tickets")
		res, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		id := res.Session.ID
		conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
		waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
		report(t, conv, agentfake.Control{State: "idle"})
		waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
		if link, err := f.h.Client.Sessions.Link(f.ctx, wire.LinkSessionRequest{ID: id, Flow: "triage", FlowKey: "proj"}); err != nil || link.Refused {
			t.Fatalf("link = %+v, %v", link, err)
		}
		other := handoffFixture{designsFixture: f.designsFixture, id: id, conv: conv}
		other.continueSession(t)
		wantResetInPlace(t, other, "worker")
	})
}

// wantResetInPlace waits for the fake agent to move to a new conversation on
// /clear, then checks the session never got a new holder run.
func wantResetInPlace(t *testing.T, f handoffFixture, tier string) {
	t.Helper()
	meta := f.newConversation(t, f.conv)
	if meta["run"] != nil || meta["tier"] != tier {
		t.Errorf("meta run=%v tier=%v, want the same run on %s", meta["run"], meta["tier"], tier)
	}
}

func TestRestartFresh_KeepsIdAndTierAndTypesThePromptAgain(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, wire.CreateSessionRequest{Ticket: "proj:epic/01", Prompt: "fix the bug"})
	dir := filepath.Join(h.SessionsDir, id)
	report(t, conv, agentfake.Control{State: "done"})
	waitForScreen(t, waitForHolder(t, dir), "fix the bug")
	before := readMeta(t, h, id)
	cwd, _ := before["dir"].(string)
	writeTranscriptTurns(t, h, cwd, conv, 100, 200)
	if _, ok := before["first_prompt"]; ok {
		t.Fatal("first prompt still saved after it was typed")
	}
	if _, ok := before["vim_relaunch_run"]; ok {
		t.Fatal("vim_relaunch_run set before any restart")
	}

	server.ResumerOf(h.Server).RestartFresh(dir, id, "fix the bug again")

	after := readMeta(t, h, id)
	if after["id"] != id || after["tier"] != before["tier"] {
		t.Errorf("id/tier = %v/%v, want %v/%v", after["id"], after["tier"], id, before["tier"])
	}
	if after["conversation_id"] == conv || after["conversation_id"] == "" {
		t.Errorf("conversation_id = %v, want a new one (was %s)", after["conversation_id"], conv)
	}
	run := after["run"].(float64)
	if prev, _ := before["run"].(float64); run <= prev {
		t.Errorf("run = %v, want above %v", run, prev)
	}
	if after["vim_relaunch_run"] != run {
		t.Errorf("vim_relaunch_run = %v, want %v", after["vim_relaunch_run"], run)
	}
	if first := recordsByID(t, h.ConversationsDir)[conv].Totals; first == nil || first.TotalTokens != 320 {
		t.Errorf("old conversation totals = %+v, want 320 tokens", first)
	}
	// The new watch types the handed-back prompt into the new agent once it is idle.
	report(t, after["conversation_id"].(string), agentfake.Control{State: "done"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle || s.Status.State == wire.SessionWorking
	})
	waitForScreen(t, waitForHolder(t, dir), "fix the bug again")
}

func TestLinkSession_TriageSetsFlowAndKey(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())
	ctx := context.Background()
	newSession := func(ticket string) string {
		t.Helper()
		created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || created.Session == nil {
			t.Fatalf("create = %+v, %v", created, err)
		}
		return created.Session.ID
	}

	t.Run("plain session", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession(""), Flow: "triage", FlowKey: "proj"})
		if err != nil || res.Refused || res.Session == nil {
			t.Fatalf("link = %+v, %v", res, err)
		}
		if s := res.Session; s.Flow != "triage" || s.FlowKey != "proj" || s.Ticket != "" || s.Design {
			t.Errorf("session = %+v; want flow triage, key proj (unqualified), no ticket, not design", s)
		}
	})

	t.Run("unknown project", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession(""), Flow: "triage", FlowKey: "nope"})
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
			t.Errorf("link = %+v, %v; want refused %s", res, err, wire.ReasonSessionBadRequest)
		}
	})

	t.Run("design session", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession("proj:map"), Flow: "triage", FlowKey: "proj"})
		if err != nil || !res.Refused {
			t.Errorf("link = %+v, %v; want refused on a Design session", res, err)
		}
	})
}
