package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// resumeSpacing mirrors the server's gap between a start's auto-resumes.
const resumeSpacing = 100 * time.Millisecond

// linkedHolder is the test holder reached through a symlink, so a test can
// make the holder start fail by removing the link and fix it by relinking.
type linkedHolder struct{ link string }

func newLinkedHolder(t *testing.T) *linkedHolder {
	t.Helper()
	l := &linkedHolder{link: filepath.Join(t.TempDir(), "holder")}
	l.fix(t)
	return l
}

func (l *linkedHolder) opt(c *server.Config) {
	servertest.WithTestHolder()(c)
	c.Holder = append([]string{l.link}, c.Holder[1:]...)
}

func (l *linkedHolder) fix(t *testing.T) {
	t.Helper()
	if err := os.Symlink(os.Args[0], l.link); err != nil {
		t.Fatal(err)
	}
}

func (l *linkedHolder) brk(t *testing.T) {
	t.Helper()
	if err := os.Remove(l.link); err != nil {
		t.Fatal(err)
	}
}

// assertFailedStartEnded checks a session whose holder failed to start is
// Exited crashed, stays so across a restart with auto-resume on, and can be
// removed.
func assertFailedStartEnded(ctx context.Context, t *testing.T, h *servertest.Harness, holder *linkedHolder, id string, wantRun any) {
	t.Helper()
	got := waitForSnapshot(ctx, t, h, id, func(wire.SessionInfo) bool { return true })
	if got.Status.State != wire.SessionExited || got.Exit != wire.SessionExitCrashed {
		t.Fatalf("session = %+v, want exited crashed", got)
	}

	holder.fix(t)
	h.Restart(t)
	// Negative wait: auto-resume runs after the restart returns; give it the
	// time it would take to bump the run.
	time.Sleep(3 * resumeSpacing)
	got = waitForSnapshot(ctx, t, h, id, func(wire.SessionInfo) bool { return true })
	if got.Status.State != wire.SessionExited {
		t.Errorf("after restart = %+v, want still exited", got)
	}
	if run := readMeta(t, h, id)["run"]; run != wantRun {
		t.Errorf("run = %v, want %v: no auto-resume", run, wantRun)
	}

	if res, err := h.Client.Sessions.Remove(ctx, id); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(h.SessionsDir, id)); !os.IsNotExist(err) {
		t.Errorf("state dir after remove: %v, want gone", err)
	}
}

func TestCreateSession_FailedHolderStartEndsCrashed(t *testing.T) {
	t.Parallel()
	holder := newLinkedHolder(t)
	h := servertest.StartWithStore(t, t.TempDir(), resumeOnStart, holder.opt)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	holder.brk(t)

	if res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)}); err == nil {
		t.Fatalf("create = %+v, want an error", res)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil || len(snap.Sessions) != 1 {
		t.Fatalf("snapshot = %+v, %v; want one session", snap.Sessions, err)
	}
	assertFailedStartEnded(ctx, t, h, holder, snap.Sessions[0].ID, nil)
}

// switchFailsAndRollsBack switches the idle session to model, lets the new agent
// exit before it is idle, and brings the old agent back to idle.
func switchFailsAndRollsBack(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan wire.Event, id, conv, model string) wire.SessionInfo {
	t.Helper()
	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: model, Effort: "high"}); err != nil || res.Refused {
		t.Fatalf("switch = %+v, %v", res, err)
	}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit, Code: 1}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.SwitchingBack })
	report(t, conv, agentfake.Control{State: "idle"})
	return waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle && s.SwitchFailedModel != ""
	})
}

func TestSwitchSession_NewAgentExitingBeforeIdleRollsBackToTheOldRecord(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startIdleSwitchable(t)
	before := readMeta(t, h, id)

	got := switchFailsAndRollsBack(ctx, t, h, evs, id, conv, "opus")
	if got.ID != id || got.SwitchFailedModel != "opus" || got.SwitchFailedOld != before["model"] || got.SwitchingModel != "" {
		t.Errorf("session = %+v, want a failed switch to opus still on %v", got, before["model"])
	}
	after := readMeta(t, h, id)
	for _, k := range []string{"model", "effort", "tier", "switched_by_hand", "switch_unconfirmed", "switch_prev", "conversation_id"} {
		if !reflect.DeepEqual(after[k], before[k]) {
			t.Errorf("meta[%s] = %v, want %v as before the switch", k, after[k], before[k])
		}
	}
}

func TestSwitchSession_FailedSwitchKeepsAnEarlierHandSwitch(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startIdleSwitchable(t)
	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: "opus", Effort: "high"}); err != nil || res.Refused {
		t.Fatalf("first switch = %+v, %v", res, err)
	}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle && s.SwitchingModel == "" && readMeta(t, h, id)["model"] == "opus"
	})
	before := readMeta(t, h, id)

	got := switchFailsAndRollsBack(ctx, t, h, evs, id, conv, "sonnet")
	if got.SwitchFailedModel != "sonnet" || got.SwitchFailedOld != "opus" {
		t.Errorf("session = %+v, want a failed switch to sonnet still on opus", got)
	}
	after := readMeta(t, h, id)
	if after["model"] != "opus" || after["switched_by_hand"] != true || !reflect.DeepEqual(after["switch_prev"], before["switch_prev"]) {
		t.Errorf("meta = %v, want the record of the earlier hand switch %v", after, before)
	}
}

func TestSwitchSession_FailedHolderStartAndFailedRollbackEndCrashed(t *testing.T) {
	t.Parallel()
	holder := newLinkedHolder(t)
	h, ctx, _, id, _ := startIdleSwitchable(t, resumeOnStart, holder.opt)
	holder.brk(t)

	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: "opus"}); err == nil {
		t.Fatalf("switch = %+v, want an error", res)
	}
	assertFailedStartEnded(ctx, t, h, holder, id, readMeta(t, h, id)["run"])
}

func TestResumeSession_FailedHolderStartEndsCrashed(t *testing.T) {
	t.Parallel()
	holder := newLinkedHolder(t)
	h, ctx, evs, id, _ := startFakeSessionWith(t, wire.CreateSessionRequest{}, resumeOnStart, holder.opt)
	stopFakeSession(ctx, t, h, evs, id)
	holder.brk(t)

	if res, err := h.Client.Sessions.Resume(ctx, id); err == nil {
		t.Fatalf("resume = %+v, want an error", res)
	}
	assertFailedStartEnded(ctx, t, h, holder, id, 1.0)
}
