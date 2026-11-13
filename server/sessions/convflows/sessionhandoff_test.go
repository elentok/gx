package convflows_test

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

func TestChartingHandoff_ContinueRestartsUnderWorkerArgs(t *testing.T) {
	t.Parallel()
	f := rigs.StartHandoff(t, "chart")
	before := readMeta(t, f.H, f.ID)
	if before["tier"] != "strongest" {
		t.Fatalf("tier = %v, want strongest", before["tier"])
	}
	setMapStep(t, f.Store, "resolve-tickets")
	f.ContinueSession(t)
	f.WantRestartedOnWorker(t)
	if after := readMeta(t, f.H, f.ID); after["name"] != before["name"] {
		t.Errorf("name = %v, want %v", after["name"], before["name"])
	}
}

func TestChartingHandoff_AutoAdvanceRestartsUnderWorkerArgs(t *testing.T) {
	t.Parallel()
	f := rigs.StartHandoff(t, "chart", servertest.WithAutoAdvanceDelay(200*time.Millisecond))
	setMapStep(t, f.Store, "resolve-tickets")
	writeAssistantMessage(t, filepath.Join(f.Cfg, "projects", "-repo", f.Conv+".jsonl"), runEndText)
	rigs.EndTurn(t, &rigs.GrillingFixture{H: f.H, Ctx: f.Ctx, Evs: f.Evs, ID: f.ID, Conv: f.Conv})
	f.WantRestartedOnWorker(t)
}

func TestChartingHandoff_FailedRestartExitsCrashedAndResumeWorks(t *testing.T) {
	t.Parallel()
	f := rigs.StartHandoff(t, "chart")
	dir := readMeta(t, f.H, f.ID)["dir"].(string)
	setMapStep(t, f.Store, "resolve-tickets")
	setWorkerModel(t, f.Store, "stale-model")
	// A session dir that is gone stands in for a stale worker slug: the new
	// agent cannot start.
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	f.ContinueSession(t)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionExited && s.Exit == wire.SessionExitCrashed
	})
	if err := os.Rename(moved, dir); err != nil {
		t.Fatal(err)
	}
	if model := readMeta(t, f.H, f.ID)["model"]; model != "stale-model" {
		t.Fatalf("model after the failed restart = %v, want stale-model", model)
	}
	setWorkerModel(t, f.Store, "sonnet")
	res, err := f.H.Client.Sessions.Resume(f.Ctx, f.ID)
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State != wire.SessionExited })
	if model := readMeta(t, f.H, f.ID)["model"]; model != "sonnet" {
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
		f := rigs.StartHandoff(t, "resolve-tickets")
		f.ContinueSession(t)
		wantResetInPlace(t, f, "worker")
	})
	t.Run("still charting", func(t *testing.T) {
		f := rigs.StartHandoff(t, "chart")
		f.ContinueSession(t)
		wantResetInPlace(t, f, "strongest")
	})
	t.Run("linked later", func(t *testing.T) {
		f := rigs.StartHandoff(t, "chart")
		setMapStep(t, f.Store, "resolve-tickets")
		// A session linked to the design later has no launch kind.
		res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		id := res.Session.ID
		conv, _ := readMeta(t, f.H, id)["conversation_id"].(string)
		waitForHolder(t, filepath.Join(f.H.SessionsDir, id))
		report(t, conv, agentfake.Control{State: "idle"})
		waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
		other := f
		other.ID, other.Conv = id, conv
		other.ContinueSession(t)
		wantResetInPlace(t, other, "worker")
	})
	t.Run("triage session", func(t *testing.T) {
		f := rigs.StartHandoff(t, "chart")
		setMapStep(t, f.Store, "resolve-tickets")
		res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		id := res.Session.ID
		conv, _ := readMeta(t, f.H, id)["conversation_id"].(string)
		waitForHolder(t, filepath.Join(f.H.SessionsDir, id))
		report(t, conv, agentfake.Control{State: "idle"})
		waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
		if link, err := f.H.Client.Sessions.Link(f.Ctx, wire.LinkSessionRequest{ID: id, Flow: "triage", FlowKey: "proj"}); err != nil || link.Refused {
			t.Fatalf("link = %+v, %v", link, err)
		}
		other := f
		other.ID, other.Conv = id, conv
		other.ContinueSession(t)
		wantResetInPlace(t, other, "worker")
	})
}

// wantResetInPlace waits for the fake agent to move to a new conversation on
// /clear, then checks the session never got a new holder run.
func wantResetInPlace(t *testing.T, f rigs.HandoffFixture, tier string) {
	t.Helper()
	meta := f.NewConversation(t, f.Conv)
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
