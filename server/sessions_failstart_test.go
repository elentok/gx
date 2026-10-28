package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	got := waitForSnapshot(ctx, t, h, id, func(server.SessionInfo) bool { return true })
	if got.Status.State != server.SessionExited || got.Exit != server.SessionExitCrashed {
		t.Fatalf("session = %+v, want exited crashed", got)
	}

	holder.fix(t)
	h.Restart(t)
	// Auto-resume runs after the restart returns; give it the time it would
	// take to bump the run.
	time.Sleep(3 * resumeSpacing)
	got = waitForSnapshot(ctx, t, h, id, func(server.SessionInfo) bool { return true })
	if got.Status.State != server.SessionExited {
		t.Errorf("after restart = %+v, want still exited", got)
	}
	if run := readMeta(t, h, id)["run"]; run != wantRun {
		t.Errorf("run = %v, want %v: no auto-resume", run, wantRun)
	}

	if res, err := h.Client.RemoveSession(ctx, id); err != nil || res.Refused {
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

	if res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)}); err == nil {
		t.Fatalf("create = %+v, want an error", res)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil || len(snap.Sessions) != 1 {
		t.Fatalf("snapshot = %+v, %v; want one session", snap.Sessions, err)
	}
	assertFailedStartEnded(ctx, t, h, holder, snap.Sessions[0].ID, nil)
}

func TestResumeSession_FailedHolderStartEndsCrashed(t *testing.T) {
	t.Parallel()
	holder := newLinkedHolder(t)
	h, ctx, evs, id, _ := startFakeSessionWith(t, server.CreateSessionRequest{}, resumeOnStart, holder.opt)
	stopFakeSession(ctx, t, h, evs, id)
	holder.brk(t)

	if res, err := h.Client.ResumeSession(ctx, id); err == nil {
		t.Fatalf("resume = %+v, want an error", res)
	}
	assertFailedStartEnded(ctx, t, h, holder, id, 1.0)
}
