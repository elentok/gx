package server_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const codexPromptConv = "thread-server-prompt"

// startCodexPromptSession runs a session on the fake Codex, Idle.
func startCodexPromptSession(t *testing.T) promptFixture {
	t.Helper()
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "codex"), "github.com/elentok/gx/testutil/agentfake/cmd/fakecodex").CombinedOutput(); err != nil {
		t.Fatalf("build fakecodex: %v\n%s", err, out)
	}
	home := t.TempDir()
	f := startDesigns(t, func(c *server.Config) { c.PromptGrace = firstPromptGrace },
		servertest.WithChatHarness("codex"),
		servertest.WithEnv(
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"HOME="+home,
			"CODEX_HOME="+filepath.Join(home, ".codex"),
			agentfake.CodexIDEnv+"="+codexPromptConv,
			agentfake.ProgramDirEnv+"="+agentfake.NewProgramDir(t),
		))
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: "codex"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	return promptFixture{designsFixture: f, id: id, conv: codexPromptConv}
}

func TestSessionPrompt_CodexIsRefusedWhileWorkingAndTypedAtIdle(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	if res := f.prompt(t, "line one\nline two"); res.Refused {
		t.Fatalf("prompt at idle = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("line one\nline two")+"\n")
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	if res := f.prompt(t, "again"); !res.Refused || res.Reason != server.ReasonSessionNotIdle {
		t.Fatalf("prompt while working = %+v, want %s", res, server.ReasonSessionNotIdle)
	}
}
