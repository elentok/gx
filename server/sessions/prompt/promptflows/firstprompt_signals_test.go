package promptflows_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

const signalPrompt = "fix the bug"

// startTUIPromptSession starts a claude session on the TUI fake with a plain
// first prompt. It returns the server log path.
func startTUIPromptSession(t *testing.T) string {
	t.Helper()
	bin, home := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nexport " + agentfake.TUIEnv + "=1\nexec '" + os.Args[0] + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(),
		servertest.WithEnv("PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+home, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude")))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: "claude", Prompt: signalPrompt})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	return server.LogPath(h.StateDir)
}

// Story 4, pid-file idle: the prompt starts on the pid file's report, once.
// One test func holds the four first-prompt flows: the package is at its
// Test-func limit.
func TestFirstPromptSignal(t *testing.T) {
	t.Parallel()
	t.Run("PidFileIdleStartsThePromptOnce", pidFileIdleStartsThePromptOnce)
	t.Run("OscIdleStartsThePromptOnce", oscIdleStartsThePromptOnce)
	t.Run("ClientAttachedBeforeThePromptSendsNoClearAndNoDraft", clientAttachedBeforeThePromptSendsNoClearAndNoDraft)
	t.Run("PrefilledPromptGetsOnlyEnter", prefilledPromptGetsOnlyEnter)
}

func pidFileIdleStartsThePromptOnce(t *testing.T) {
	t.Parallel()
	logPath := startTUIPromptSession(t)
	var log string
	servertest.WaitFor(t, 20*time.Second, "the first prompt typed", func() bool {
		data, _ := os.ReadFile(logPath)
		log = string(data)
		return strings.Contains(log, "first prompt typed")
	})
	time.Sleep(time.Second) // negative wait: a second start or a nudge would come within a few grace windows
	data, _ := os.ReadFile(logPath)
	log = string(data)
	for msg, want := range map[string]int{"first prompt ready": 1, "first prompt typed": 1, "first prompt nudge": 0} {
		if n := strings.Count(log, msg); n != want {
			t.Errorf("%q lines = %d, want %d", msg, n, want)
		}
	}
	if !strings.Contains(log, `"signal":"pid"`) {
		t.Error("server log lacks the pid signal")
	}
}

// Story 4, OSC 7501 idle: the fake's idle report is the ready signal, and the
// prompt reaches it once.
func oscIdleStartsThePromptOnce(t *testing.T) {
	t.Parallel()
	h, _, _, _, conv := servertest.StartFakeSessionWith(t, wire.CreateSessionRequest{Prompt: signalPrompt}, func(c *server.Config) { c.PromptGrace = firstPromptGrace },
		servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\""+signalPrompt+"\"\n")
	report(t, conv, agentfake.Control{State: "working"}) // the turn started: no nudge follows
	time.Sleep(2 * firstPromptGrace)                     // negative wait: a repeat would come within the grace window
	if got := submitted(t, conv); got != "\""+signalPrompt+"\"\n" {
		t.Errorf("submitted = %q, want exactly one submission", got)
	}
	data, _ := os.ReadFile(server.LogPath(h.StateDir))
	if !strings.Contains(string(data), `"signal":"osc7501"`) {
		t.Error("server log lacks the osc7501 signal")
	}
}

// Story 6: a client attached before the first prompt is not the owner while the
// prompt's batch runs, so the fake receives the prompt alone: no clear key and
// none of the client's draft.
func clientAttachedBeforeThePromptSendsNoClearAndNoDraft(t *testing.T) {
	t.Parallel()
	h, _, _, id, conv := servertest.StartFakeSessionWith(t, wire.CreateSessionRequest{Prompt: signalPrompt}, func(c *server.Config) { c.PromptGrace = firstPromptGrace },
		servertest.WithEnv(agentfake.ProgramInputEnv+"=1", agentfake.ProgramLogClearEnv+"=1"))
	holder := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	a, err := holder.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	report(t, conv, agentfake.Control{State: "idle"})
	_ = a.Paste("my draft") // may be refused while gx owns the input; either way it must not reach the fake
	waitForSubmitted(t, conv, "\""+signalPrompt+"\"\n")
}

// Story 8: a prefilled prompt already sits in the input, so only Enter is sent.
func prefilledPromptGetsOnlyEnter(t *testing.T) {
	t.Parallel()
	h, _, _, id, conv := servertest.StartFakeSessionWith(t, wire.CreateSessionRequest{Prompt: signalPrompt}, func(c *server.Config) { c.PromptGrace = firstPromptGrace },
		servertest.WithEnv(agentfake.ProgramInputEnv+"=1", agentfake.ProgramLogClearEnv+"=1", agentfake.ProgramDraftEnv+"="+signalPrompt))
	dir := filepath.Join(h.SessionsDir, id)
	waitForHolder(t, dir)
	meta, err := sessions.ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	meta.FirstPrefilled = true
	if err := sessions.WriteMeta(dir, meta); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	// Typing on top of the draft would log it twice; a clear would log a clear key.
	waitForSubmitted(t, conv, "\""+signalPrompt+"\"\n")
}
