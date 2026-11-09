package prompt_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// triageSession is an idle plain session linked to proj's Triage, on the
// fixture of a Design session.
func triageSession(t *testing.T) (servertest.PromptFixture, string, string) {
	t.Helper()
	f := servertest.StartPromptSession(t, servertest.FirstPromptGrace)
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, f.H, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.H.SessionsDir, id))
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.Ctx, t, f.H, f.Evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	link, err := f.H.Client.Sessions.Link(f.Ctx, wire.LinkSessionRequest{ID: id, Flow: "triage", FlowKey: "proj"})
	if err != nil || link.Refused {
		t.Fatalf("link = %+v, %v", link, err)
	}
	return f, id, conv
}

func TestTriageSession_ContinueTypesClearThenTheTriageSkill(t *testing.T) {
	t.Parallel()
	f, id, conv := triageSession(t)
	res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: id, Continue: true})
	if err != nil || res.Refused {
		t.Fatalf("continue = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, "\"/clear\"\n\"/gx-triage-follow-ups --project proj\"\n")
}

func TestTriageSession_AutoAdvanceSwitchFlips(t *testing.T) {
	t.Parallel()
	f, id, _ := triageSession(t)
	for _, on := range []bool{false, true} {
		res, err := f.H.Client.Sessions.SetAutoAdvance(f.Ctx, id, on)
		if err != nil || res.Refused || res.Session == nil || res.Session.AutoAdvance != on {
			t.Fatalf("set auto-advance %v = %+v, %v", on, res, err)
		}
	}
}

func TestTriageSession_PromoteIsRefused(t *testing.T) {
	t.Parallel()
	f, id, _ := triageSession(t)
	res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: id, Promote: true})
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotPlain {
		t.Errorf("promote = %+v, %v; want %s", res, err, wire.ReasonSessionNotPlain)
	}
}

// triageRunEnd ends a turn whose last message is the run-end line and waits
// for the Actionable that line raises.
func triageRunEnd(t *testing.T, step string, done bool) *grillingFixture {
	t.Helper()
	f, _ := servertest.StartTriageSession(t, step, done)
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	return f
}

func TestTriageRunEnd_RaisesContinue(t *testing.T) {
	t.Parallel()
	t.Run("verify auto-advances", func(t *testing.T) {
		t.Parallel()
		f := triageRunEnd(t, "verify", false)
		got := sessionSnapshot(t, f)
		if len(got.Actionable.Action) != 2 || got.Actionable.Action[1] != "/gx-triage-follow-ups --project proj" {
			t.Fatalf("action = %q, want /clear then the Triage skill", got.Actionable.Action)
		}
		waitForSubmitted(t, f.Conv, "\"/clear\"\n\"/gx-triage-follow-ups --project proj\"\n")
	})
	t.Run("write with pending rows", func(t *testing.T) {
		t.Parallel()
		f := triageRunEnd(t, "write", false)
		if got := sessionSnapshot(t, f); len(got.Actionable.Action) == 0 {
			t.Fatalf("actionable = %+v, want a Continue", got.Actionable)
		}
	})
}

func TestTriageRunEnd_FinalStepsRaiseNoContinueAndNoCountdown(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, step string
		done       bool
	}{
		{"plan run end, step already approve", "approve", false},
		{"write with no pending rows", "write", true},
		{"done", "done", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := triageRunEnd(t, tc.step, tc.done)
			got := sessionSnapshot(t, f)
			if len(got.Actionable.Action) != 0 || got.AutoAdvanceAt != nil {
				t.Fatalf("actionable = %+v, advance at %v, want the line alone", got.Actionable, got.AutoAdvanceAt)
			}
			time.Sleep(2 * servertest.CountdownDelay) // negative wait: a countdown would have sent by now
			if n := servertest.AutoSends(t, f); n != 0 {
				t.Errorf("auto sends = %d, want none", n)
			}
		})
	}
}
