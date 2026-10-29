package server_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
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
	cfg := t.TempDir()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	setMapStep(t, store, step)
	f := startDesignsIn(t, store, append(opts, servertest.WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+cfg))...)
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	return handoffFixture{designsFixture: f, store: store, cfg: cfg, id: id, conv: conv}
}

func setMapStep(t *testing.T, store, step string) {
	t.Helper()
	body := "---\nkind: map\ndesign_step: " + step + "\n---\n"
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f handoffFixture) continueSession(t *testing.T) {
	t.Helper()
	res, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: f.id, Continue: true})
	if err != nil || res.Refused {
		t.Fatalf("continue = %+v, %v", res, err)
	}
}

// newConversation waits for the session to move off conv and returns its meta.
func (f handoffFixture) newConversation(t *testing.T, old string) map[string]any {
	t.Helper()
	var meta map[string]any
	servertest.WaitFor(t, 10*time.Second, "a new conversation id", func() bool {
		meta = readMeta(t, f.h, f.id)
		return meta["conversation_id"] != old
	})
	return meta
}

func (f handoffFixture) wantRestartedOnWorker(t *testing.T) {
	t.Helper()
	meta := f.newConversation(t, f.conv)
	newConv, _ := meta["conversation_id"].(string)
	if meta["tier"] != "worker" || meta["launch_design"] != true || meta["ticket"] != "proj:map" {
		t.Errorf("meta tier=%v launch_design=%v ticket=%v, want worker, true, proj:map", meta["tier"], meta["launch_design"], meta["ticket"])
	}
	for _, c := range []string{f.conv, newConv} {
		if _, err := conversations.Read(f.h.ConversationsDir, c); err != nil {
			t.Errorf("conversation %s not recorded: %v", c, err)
		}
	}
	waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	report(t, newConv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, newConv, "\"/gx-design map\"\n")
	snap := sessionSnapshotOf(t, f.designsFixture, f.id)
	if snap.Status.State == server.SessionExited {
		t.Errorf("session = %+v, want it alive", snap)
	}
}

func sessionSnapshotOf(t *testing.T, f designsFixture, id string) server.SessionInfo {
	t.Helper()
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not in the snapshot", id)
	return server.SessionInfo{}
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
	endTurn(t, &grillingFixture{h: f.h, ctx: f.ctx, evs: f.evs, id: f.id, conv: f.conv})
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
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool {
		return s.Status.State == server.SessionExited && s.Exit == server.SessionExitCrashed
	})
	if err := os.Rename(moved, dir); err != nil {
		t.Fatal(err)
	}
	if model := readMeta(t, f.h, f.id)["model"]; model != "stale-model" {
		t.Fatalf("model after the failed restart = %v, want stale-model", model)
	}
	setWorkerModel(t, f.store, "sonnet")
	res, err := f.h.Client.ResumeSession(f.ctx, f.id)
	if err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State != server.SessionExited })
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
		res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		id := res.Session.ID
		conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
		waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
		report(t, conv, agentfake.Control{State: "idle"})
		waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
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
