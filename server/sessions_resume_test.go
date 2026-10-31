package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// stopFakeSession stops a session and waits for it to exit.
func stopFakeSession(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id string) {
	t.Helper()
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if res, err := h.Client.StopSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
}

func TestResumeSession_KeepsSessionOnSavedConversation(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	idleWithAction(ctx, t, h, evs, id, conv, "/review")
	stopFakeSession(ctx, t, h, evs, id)

	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	got := res.Session
	if got.ID != id || got.Name != "proj-1" || got.Status.State != server.SessionStarting || got.Stopped || got.Exit != "" {
		t.Errorf("resumed session = %+v, want %s proj-1 starting, not stopped", got, id)
	}
	if got.Actionable.Reason != "" {
		t.Errorf("actionable = %+v, want the exit to have cleared it", got.Actionable)
	}

	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
}

func TestResumeSession_UsesNewestConversation(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	// Unique, since the fake's control FIFO is named after it.
	newer := conv + "-cleared"
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionConversation, ConversationID: newer}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(server.SessionInfo) bool { return readMeta(t, h, id)["conversation_id"] == newer })
	stopFakeSession(ctx, t, h, evs, id)

	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	report(t, newer, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
}

// Restore of an Exited Codex session needs the conversation's rollout: without
// it a fresh Codex would lose the conversation silently.
func TestResumeSession_CodexNeedsItsRollout(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	h, ctx, id, conv, dir := stoppedCodexSession(t, servertest.WithEnv("CODEX_HOME="+codexHome))

	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionNoResume || !strings.Contains(res.Message, "rollout") {
		t.Fatalf("resume without rollout = %+v, %v; want refused %s naming the rollout", res, err, server.ReasonSessionNoResume)
	}

	writeCodexRollout(t, codexHome, conv, dir)
	// Past the rollout check; a host without codex still refuses, for the missing binary.
	res, err = h.Client.ResumeSession(ctx, id)
	if err != nil || res.Reason == server.ReasonSessionNoResume {
		t.Errorf("resume with rollout = %+v, %v; want it past the rollout check", res, err)
	}
}

// stoppedCodexSession is an Exited session whose meta says it ran on Codex.
func stoppedCodexSession(t *testing.T, opts ...func(*server.Config)) (h *servertest.Harness, ctx context.Context, id, conv, dir string) {
	t.Helper()
	h, ctx, evs, id, conv := startFakeSession(t, opts...)
	stopFakeSession(ctx, t, h, evs, id)
	meta := readMeta(t, h, id)
	meta["harness"] = "codex"
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _ = meta["dir"].(string)
	return h, ctx, id, conv, dir
}

// writeCodexRollout gives conv a rollout under codexHome, as Codex writes one for a session in dir.
func writeCodexRollout(t *testing.T, codexHome, conv, dir string) {
	t.Helper()
	rollouts := filepath.Join(codexHome, "sessions", "2026", "10", "30")
	if err := os.MkdirAll(rollouts, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"session_meta","payload":{"id":` + strconv.Quote(conv) + `,"cwd":` + strconv.Quote(dir) + `}}` + "\n"
	if err := os.WriteFile(filepath.Join(rollouts, "rollout-2026-10-30T00-00-00-"+conv+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Restore of an Exited interactive Codex session runs `codex resume <conv> --cd <dir>`.
func TestResumeSession_CodexLaunchesCodexResume(t *testing.T) {
	t.Parallel()
	bin, argsFile, codexHome := t.TempDir(), filepath.Join(t.TempDir(), "args"), t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_CODEX_ARGS\"\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h, ctx, id, conv, dir := stoppedCodexSession(t, servertest.WithEnv(
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CODEX_HOME="+codexHome,
		"FAKE_CODEX_ARGS="+argsFile,
	))
	writeCodexRollout(t, codexHome, conv, dir)

	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	var args []string
	waitFor(t, func() bool {
		data, err := os.ReadFile(argsFile)
		args = strings.Fields(string(data))
		return err == nil && len(args) > 0
	})
	if len(args) < 4 || !slices.Equal(args[:4], []string{"resume", conv, "--cd", dir}) {
		t.Errorf("codex args = %q, want to start with resume %s --cd %s", args, conv, dir)
	}
}

func TestResumeSession_Refusals(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSession(t)
	if res, err := h.Client.ResumeSession(ctx, id); err != nil || !res.Refused || res.Reason != server.ReasonSessionLive {
		t.Errorf("resume live = %+v, %v; want refused %s", res, err, server.ReasonSessionLive)
	}

	noResume, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind)})
	if err != nil || noResume.Session == nil {
		t.Fatalf("create = %+v, %v", noResume, err)
	}
	stopFakeSession(ctx, t, h, evs, noResume.Session.ID)
	if res, err := h.Client.ResumeSession(ctx, noResume.Session.ID); err != nil || !res.Refused || res.Reason != server.ReasonSessionNoResume {
		t.Errorf("resume without harness resume = %+v, %v; want refused %s", res, err, server.ReasonSessionNoResume)
	}

	stopFakeSession(ctx, t, h, evs, id)
	dir, _ := readMeta(t, h, id)["dir"].(string)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionDirGone || res.Message != "dir is gone" {
		t.Errorf("resume with dir gone = %+v, %v; want refused %s", res, err, server.ReasonSessionDirGone)
	}
}
