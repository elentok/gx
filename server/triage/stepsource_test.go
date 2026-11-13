package triage_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// stays reports whether the session is still not exited after a short wait.
func stays(t *testing.T, f *rigs.GrillingFixture, id string) bool {
	t.Helper()
	time.Sleep(500 * time.Millisecond) // negative wait: a stop would land within it
	list, err := f.H.Client.Sessions.List(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s.Status.State != wire.SessionExited
		}
	}
	return false
}

func TestStopFinished_IdleSessionAtAFinalStepIsStopped(t *testing.T) {
	t.Parallel()
	f, _ := rigs.StartTriageSession(t, "approve", false)
	rigs.EndTurn(t, f)
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
}

func TestStopFinished_NotFinalStepKeepsTheSession(t *testing.T) {
	t.Parallel()
	f, _ := rigs.StartTriageSession(t, "verify", false)
	rigs.EndTurn(t, f)
	if !stays(t, f, f.ID) {
		t.Fatal("an idle session at verify was stopped")
	}
}

func TestStopFinished_WorkingSessionIsNeverStopped(t *testing.T) {
	t.Parallel()
	f, _ := rigs.StartTriageSession(t, "approve", false)
	servertest.Report(t, f.Conv, agentfake.Control{State: "working"})
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	server.TriageOf(f.H.Server).StopFinished("proj")
	if !stays(t, f, f.ID) {
		t.Fatal("a working session was stopped")
	}
}

func TestStopFinished_AnotherProjectsTriageSessionIsNotTouched(t *testing.T) {
	t.Parallel()
	f, _ := rigs.StartTriageSession(t, "approve", false)
	other, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || other.Session == nil {
		t.Fatalf("create = %+v, %v", other, err)
	}
	conv, _ := servertest.ReadMeta(t, f.H, other.Session.ID)["conversation_id"].(string)
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, other.Session.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if res, err := f.H.Client.Sessions.Link(f.Ctx, wire.LinkSessionRequest{ID: other.Session.ID, Flow: "triage", FlowKey: "other"}); err != nil || res.Refused {
		t.Fatalf("link = %+v, %v", res, err)
	}
	rigs.EndTurn(t, f)
	servertest.WaitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if !stays(t, f, other.Session.ID) {
		t.Fatal("another project's Triage session was stopped")
	}
}
