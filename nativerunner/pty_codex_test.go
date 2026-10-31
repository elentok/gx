package nativerunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	gxharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// codexRunner returns a pty runner whose codex is the built fakecodex binary,
// and the Codex home that fake writes its rollout to.
func codexRunner(t *testing.T, env ...string) (p *nativerunner.Pty, codexHome string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "codex")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex").CombinedOutput(); err != nil {
		t.Fatalf("build fakecodex: %v\n%s", err, out)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := shortTempDir(t)
	t.Cleanup(func() { killAgents(root) })
	home := t.TempDir()
	p = ptyOn(root, exe)
	p.Env = ptyTestEnv(home, append([]string{holderEnv + "=1", agentfake.ProgramDirEnv + "=" + agentfake.NewProgramDir(t)}, env...)...)
	p.Codex = bin
	return p, filepath.Join(home, ".codex")
}

func startCodex(t *testing.T, p *nativerunner.Pty, cwd string) agentrunner.Session {
	t.Helper()
	s, err := p.Start(agentrunner.StartOptions{
		Label: "e-01", Epic: "e", Cwd: cwd, Kind: gxharness.Codex,
		Args: gxharness.CodexArgs(gxharness.CodexArgsInput{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	return s
}

func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPtyCodex_StartsIdleWithCodexsOwnSessionID(t *testing.T) {
	t.Parallel()
	p, home := codexRunner(t)
	cwd := realDir(t)
	s := startCodex(t, p, cwd)

	st, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second)
	if err != nil {
		t.Fatalf("codex never read as idle: %v", err)
	}
	if st.SessionID == "" || st.SessionID == s.SessionID {
		t.Fatalf("Status.SessionID = %q, want the id Codex reports, not gx's start id %q", st.SessionID, s.SessionID)
	}
	if ok, err := codexsession.VerifyIdentityIn(home, cwd, st.SessionID); err != nil || !ok {
		t.Fatalf("Status.SessionID %q has no rollout under %s for %s (ok=%v, err=%v)", st.SessionID, home, cwd, ok, err)
	}
}

func TestPtyCodex_AnswersEveryStartupDialog(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t,
		agentfake.CodexTrustEnv+"=1",
		agentfake.CodexHooksReviewEnv+"=1",
		agentfake.CodexUpdateEnv+"=1",
		agentfake.CodexModelNudgeEnv+"=1",
	)
	// Start returns only once the launch loop saw Ready, which the fake reports
	// after the last dialog.
	s := startCodex(t, p, realDir(t))
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatalf("a startup dialog left the session paused: %v", err)
	}
}

func TestPtyCodex_WorkingWithReadyTitleIsBackground(t *testing.T) {
	t.Parallel()
	p, _ := codexRunner(t, agentfake.CodexIDEnv+"=thread-bg")
	s := startCodex(t, p, realDir(t))
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, c := range []agentfake.Control{
		{Action: agentfake.ActionReport, State: "working"},
		{Action: agentfake.ActionTitle, Title: "Ready"},
	} {
		if err := agentfake.SendProgramControl("thread-bg", c); err != nil {
			t.Fatal(err)
		}
	}
	holder, err := nativerunner.DialHolder(filepath.Join(p.Root, "e-01"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		w, err := holder.Watched()
		if err != nil {
			t.Fatal(err)
		}
		if w != nil && w.Status.State == gxharness.Working && w.Status.Background {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watched status = %+v, want Working with Background", w)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st, err := p.Status(s); err != nil || st.State == agentrunner.StateIdle {
		t.Fatalf("Status = %+v, %v; a session with a background task is not idle", st, err)
	}
}

// codexAfterError starts a codex session, sends controls then a turn error of
// the given message and kind, and returns once the runner sees it blocked.
func codexAfterError(t *testing.T, id, message, kind string, controls ...agentfake.Control) (*nativerunner.Pty, agentrunner.Session) {
	t.Helper()
	p, _ := codexRunner(t, agentfake.CodexIDEnv+"="+id)
	s := startCodex(t, p, realDir(t))
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, c := range append(controls, agentfake.Control{Action: agentfake.ActionTaskError, Message: message, Error: kind}) {
		if err := agentfake.SendProgramControl(id, c); err != nil {
			t.Fatal(err)
		}
	}
	return p, s
}

// eventually polls cond for up to 5 seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestPtyCodex_TurnErrorIsBlockedWithTheRolloutsText(t *testing.T) {
	t.Parallel()
	p, s := codexAfterError(t, "thread-err", "model gone", "model_not_found")
	eventually(t, "the blocked status", func() bool {
		st, err := p.Status(s)
		return err == nil && st.State == agentrunner.StateBlocked
	})
	st, _ := p.Status(s)
	if st.Error != "model gone" || st.BlockedReason != "" {
		t.Fatalf("Status = %+v; want Error %q and no BlockedReason", st, "model gone")
	}
	if _, limited, err := p.RateLimit(s); err != nil || limited {
		t.Fatalf("RateLimit limited = %v, %v; a model error is not a limit", limited, err)
	}
}

func TestPtyCodex_UsageLimitErrorIsLimitedWithItsReset(t *testing.T) {
	t.Parallel()
	resetsAt := time.Now().Add(time.Hour).Unix()
	p, s := codexAfterError(t, "thread-limit", "You've hit your usage limit.", "usage_limit_exceeded",
		agentfake.Control{Action: agentfake.ActionTokenCount, UsedPercent: 100, ResetsAt: resetsAt})
	eventually(t, "the limit", func() bool {
		_, limited, err := p.RateLimit(s)
		return err == nil && limited
	})
	resetAt, _, _ := p.RateLimit(s)
	if resetAt.Unix() != resetsAt {
		t.Fatalf("RateLimit reset = %v, want %d", resetAt, resetsAt)
	}
}
