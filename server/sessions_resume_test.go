package server_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

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
	if got.ID != id || got.Name != "proj-1" || got.Status.State != wire.SessionStarting || got.Stopped || got.Exit != "" {
		t.Errorf("resumed session = %+v, want %s proj-1 starting, not stopped", got, id)
	}
	if got.Actionable.Reason != "" {
		t.Errorf("actionable = %+v, want the exit to have cleared it", got.Actionable)
	}

	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
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
	waitForSession(ctx, t, h, evs, id, func(wire.SessionInfo) bool { return readMeta(t, h, id)["conversation_id"] == newer })
	stopFakeSession(ctx, t, h, evs, id)

	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	report(t, newer, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
}

// Restore of an Exited Codex session needs the conversation's rollout: without
// it a fresh Codex would lose the conversation silently.
func TestResumeSession_CodexNeedsItsRollout(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	h, ctx, id, conv, dir := stoppedCodexSession(t, servertest.WithEnv("CODEX_HOME="+codexHome))

	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNoResume || !strings.Contains(res.Message, "rollout") {
		t.Fatalf("resume without rollout = %+v, %v; want refused %s naming the rollout", res, err, wire.ReasonSessionNoResume)
	}

	writeCodexRollout(t, codexHome, conv, dir)
	// Past the rollout check; a host without codex still refuses, for the missing binary.
	res, err = h.Client.ResumeSession(ctx, id)
	if err != nil || res.Reason == wire.ReasonSessionNoResume {
		t.Errorf("resume with rollout = %+v, %v; want it past the rollout check", res, err)
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
	if res, err := h.Client.ResumeSession(ctx, id); err != nil || !res.Refused || res.Reason != wire.ReasonSessionLive {
		t.Errorf("resume live = %+v, %v; want refused %s", res, err, wire.ReasonSessionLive)
	}

	noResume, err := h.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind)})
	if err != nil || noResume.Session == nil {
		t.Fatalf("create = %+v, %v", noResume, err)
	}
	stopFakeSession(ctx, t, h, evs, noResume.Session.ID)
	if res, err := h.Client.ResumeSession(ctx, noResume.Session.ID); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNoResume {
		t.Errorf("resume without harness resume = %+v, %v; want refused %s", res, err, wire.ReasonSessionNoResume)
	}

	stopFakeSession(ctx, t, h, evs, id)
	dir, _ := readMeta(t, h, id)["dir"].(string)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.ResumeSession(ctx, id)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionDirGone || res.Message != "dir is gone" {
		t.Errorf("resume with dir gone = %+v, %v; want refused %s", res, err, wire.ReasonSessionDirGone)
	}
}
