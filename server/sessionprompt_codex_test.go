package server_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startCodexPromptSession runs a session on the fake Codex, Idle, with extraEnv
// added to the env it runs in. The conversation id is per test: the fake's
// control FIFO is keyed by it, so parallel tests sharing one id would steal
// each other's controls.
func startCodexPromptSession(t *testing.T, extraEnv ...string) promptFixture {
	t.Helper()
	return startCodexPromptSessionWithGrace(t, firstPromptGrace, extraEnv...)
}

// startCodexPromptSessionWithGrace is startCodexPromptSession with a longer
// wait for an answer to take effect, for a test whose fake does real work first.
func startCodexPromptSessionWithGrace(t *testing.T, grace time.Duration, extraEnv ...string) promptFixture {
	t.Helper()
	codexPromptConv := "thread-server-prompt-" + strings.ReplaceAll(t.Name(), "/", "-")
	bin := servertest.FakeCodexDir(t)
	home := t.TempDir()
	f := startDesigns(t, func(c *server.Config) { c.PromptGrace = grace },
		servertest.WithChatHarness("codex"),
		servertest.WithEnv(append([]string{
			"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + home,
			"CODEX_HOME=" + filepath.Join(home, ".codex"),
			agentfake.CodexIDEnv + "=" + codexPromptConv,
			agentfake.ProgramDirEnv + "=" + agentfake.NewProgramDir(t),
		}, extraEnv...)...))
	res, err := f.h.Client.CreateSession(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: "codex"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return promptFixture{designsFixture: f, id: id, conv: codexPromptConv}
}

func TestSessionPrompt_CodexIsRefusedWhileWorkingAndTypedAtIdle(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "line one\nline two"); res.Refused {
		t.Fatalf("prompt at idle = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("line one\nline two")+"\n")
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res := f.prompt(t, "again"); !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Fatalf("prompt while working = %+v, want %s", res, wire.ReasonSessionNotIdle)
	}
}

func TestSessionPrompt_CodexSkillTextKeepsTheNudgePath(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "$gx-x do it"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("$gx-x do it")+"\n")
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
}

// A Codex turn that leaves a background terminal reaches the snapshot as
// Working with Background (the Agents row shows +bg from that), and it takes a
// prompt.
func TestSessionPrompt_CodexBackgroundAgentIsMarkedAndTakesAPrompt(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "start it"); res.Refused {
		t.Fatalf("prompt at idle = %+v", res)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	for _, c := range []agentfake.Control{
		{Action: agentfake.ActionBackground, Running: true},
		{Action: agentfake.ActionFinish},
	} {
		if err := agentfake.SendProgramControl(f.conv, c); err != nil {
			t.Fatal(err)
		}
	}
	s := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.Background })
	if s.Status.State != wire.SessionWorking {
		t.Errorf("state = %s, want working with the background mark", s.Status.State)
	}
	if res := f.prompt(t, "and then"); res.Refused {
		t.Fatalf("prompt to a background agent = %+v", res)
	}
}
