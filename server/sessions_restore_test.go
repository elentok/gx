package server_test

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func resumeOnStart(c *server.Config) { c.AgentsResumeOnStart = true }

// killWhileDown stops the server, then kills the session's holder and child,
// as a reboot would.
func killWhileDown(t *testing.T, h *servertest.Harness, id string) {
	t.Helper()
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	child := killHolder(t, h, id)
	_ = syscall.Kill(-child, syscall.SIGKILL)
}

// waitForSnapshot polls the snapshot until session id satisfies ok: a
// restarted server's events may fire before a new stream subscribes.
func waitForSnapshot(ctx context.Context, t *testing.T, h *servertest.Harness, id string, ok func(server.SessionInfo) bool) server.SessionInfo {
	t.Helper()
	var last server.SessionInfo
	for {
		if snap, err := h.Client.Snapshot(ctx); err == nil {
			for _, s := range snap.Sessions {
				if s.ID == id {
					if last = s; ok(s) {
						return s
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("session %s never matched; last %+v", id, last)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRestart_ReattachesLiveHolderAndRaisesMissedDone(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSession(t)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "done"})

	h.Restart(t)
	got := waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	if !got.Unread.Unread || got.Unread.Resumed || got.Name != "proj-1" {
		t.Errorf("reattached session = %+v, want proj-1 unread, not resumed", got)
	}
	if run := readMeta(t, h, id)["run"]; run != nil {
		t.Errorf("run = %v, want the same run", run)
	}
	report(t, conv, agentfake.Control{State: "working"})
	waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
}

func TestRestart_AutoResumesDeadWorkingHolderOnce(t *testing.T) {
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, resumeOnStart)
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	killWhileDown(t, h, id)

	h.Restart(t)
	got := waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionStarting })
	if !got.Unread.Unread || !got.Unread.Resumed {
		t.Errorf("resumed session unread = %+v, want unread resumed", got.Unread)
	}
	if run := readMeta(t, h, id)["run"]; run != 1.0 {
		t.Errorf("run = %v, want 1", run)
	}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	got = waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	if !got.Unread.Resumed {
		t.Errorf("idle session unread = %+v, want the resumed marker kept", got.Unread)
	}
}

func TestRestart_ResumedChildExitingAtOnceStaysCrashed(t *testing.T) {
	h, ctx, _, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{}, resumeOnStart)
	killWhileDown(t, h, id)

	h.Restart(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionExit, Code: 0}); err != nil {
		t.Fatal(err)
	}
	got := waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if got.Exit != server.SessionExitCrashed || !got.Unread.Unread {
		t.Errorf("session = %+v, want exited crashed and unread", got)
	}
	if run := readMeta(t, h, id)["run"]; run != 1.0 {
		t.Errorf("run = %v, want 1: no second try", run)
	}
}

func TestRestart_DeadHolderWithoutAutoResumeIsCrashed(t *testing.T) {
	for name, tc := range map[string]struct {
		harness string
		opts    []func(*server.Config)
	}{
		"restore-on-start off":  {harness: string(agentfake.HarnessKind)},
		"harness cannot resume": {harness: string(agentfake.NoResumeHarnessKind), opts: []func(*server.Config){resumeOnStart}},
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx, _, id, _ := startFakeSessionWith(t, server.CreateSessionRequest{Harness: tc.harness}, tc.opts...)
			killWhileDown(t, h, id)

			h.Restart(t)
			got := waitForSnapshot(ctx, t, h, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
			if got.Exit != server.SessionExitCrashed || !got.Unread.Unread || got.Unread.Resumed {
				t.Errorf("session = %+v, want exited crashed and unread", got)
			}
			if run := readMeta(t, h, id)["run"]; run != nil {
				t.Errorf("run = %v, want no resume", run)
			}
		})
	}
}

func TestRestart_StoppedAndExitedSessionsStay(t *testing.T) {
	h, ctx, evs, stopped, _ := startFakeSessionWith(t, server.CreateSessionRequest{}, resumeOnStart)
	stopFakeSession(ctx, t, h, evs, stopped)
	exited := createFakeSession(ctx, t, h)
	exitedConv, _ := readMeta(t, h, exited)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(h.SessionsDir, exited))
	if err := agentfake.SendProgramControl(exitedConv, agentfake.Control{Action: agentfake.ActionExit, Code: 0}); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, exited, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	dead := createFakeSession(ctx, t, h)
	killWhileDown(t, h, dead)

	h.Restart(t)
	// Auto-resumes start after the scan, so by the dead one's the others are settled.
	waitForSnapshot(ctx, t, h, dead, func(s server.SessionInfo) bool { return s.Status.State == server.SessionStarting })
	for _, id := range []string{stopped, exited} {
		got := waitForSnapshot(ctx, t, h, id, func(server.SessionInfo) bool { return true })
		if got.Status.State != server.SessionExited || got.Exit != server.SessionExitClean {
			t.Errorf("session %s = %+v, want still exited clean", id, got)
		}
		if run := readMeta(t, h, id)["run"]; run != nil {
			t.Errorf("session %s run = %v, want no resume", id, run)
		}
	}
}

func createFakeSession(ctx context.Context, t *testing.T, h *servertest.Harness) string {
	t.Helper()
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	return res.Session.ID
}
