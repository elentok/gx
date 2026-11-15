package sessions_test

import (
	"context"
	"net/http"
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

	res, err := h.Client.Sessions.Resume(ctx, id)
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

func TestResumeSession_KeepsFixOf(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSessionWith(t, wire.CreateSessionRequest{FixOf: "proj:loop/01"})
	stopFakeSession(ctx, t, h, evs, id)

	res, err := h.Client.Sessions.Resume(ctx, id)
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("resume = %+v, %v", res, err)
	}
	if res.Session.FixOf != "proj:loop/01" {
		t.Errorf("resumed FixOf = %q, want proj:loop/01", res.Session.FixOf)
	}
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

	if res, err := h.Client.Sessions.Resume(ctx, id); err != nil || res.Refused {
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

	res, err := h.Client.Sessions.Resume(ctx, id)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNoResume || !strings.Contains(res.Message, "rollout") {
		t.Fatalf("resume without rollout = %+v, %v; want refused %s naming the rollout", res, err, wire.ReasonSessionNoResume)
	}

	writeCodexRollout(t, codexHome, conv, dir)
	// Past the rollout check; a host without codex still refuses, for the missing binary.
	res, err = h.Client.Sessions.Resume(ctx, id)
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

	if res, err := h.Client.Sessions.Resume(ctx, id); err != nil || res.Refused {
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

// startIdleSwitchable is a fake session whose harness counts as a chat harness,
// which is what a switch needs, brought to Idle.
func startIdleSwitchable(t *testing.T) (*servertest.Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	h, ctx, evs, id, conv := startFakeSessionWith(t, wire.CreateSessionRequest{}, servertest.WithChatHarness(string(agentfake.HarnessKind)))
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return h, ctx, evs, id, conv
}

func TestSwitchSession_RecordsOnlyOnceTheNewAgentIsIdle(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startIdleSwitchable(t)
	before := readMeta(t, h, id)

	res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: "opus", Effort: "high"})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("switch = %+v, %v", res, err)
	}
	if s := res.Session; s.ID != id || s.Stopped || s.SwitchingModel != "opus" || s.SwitchingEffort != "high" {
		t.Errorf("switch session = %+v, want %s not stopped, switching to opus high", s, id)
	}
	meta := readMeta(t, h, id)
	if meta["model"] != before["model"] || meta["effort"] != before["effort"] || meta["switched_by_hand"] != nil || meta["stopped"] != nil {
		t.Errorf("meta before idle = %v, want the old model %v, effort %v and flag", meta, before["model"], before["effort"])
	}
	if meta["conversation_id"] != conv || meta["run"] == before["run"] {
		t.Errorf("meta before idle = %v, want conversation %s resumed in a new run", meta, conv)
	}
	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: "sonnet"}); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Errorf("second switch = %+v, %v; want refused %s", res, err, wire.ReasonSessionNotIdle)
	}

	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	got := waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionIdle && s.SwitchingModel == "" && readMeta(t, h, id)["model"] == "opus"
	})
	meta = readMeta(t, h, id)
	if meta["effort"] != "high" || meta["switched_by_hand"] != true || meta["switch_unconfirmed"] != true || got.SwitchingEffort != "" {
		t.Errorf("meta after idle = %v, session %+v; want opus high recorded by hand", meta, got)
	}
}

func TestSwitchSession_Refusals(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startIdleSwitchable(t)
	req := wire.SessionSwitchRequest{ID: id, Model: "opus"}
	before := readMeta(t, h, id)
	for _, tc := range []struct {
		name   string
		req    wire.SessionSwitchRequest
		reason string
	}{
		{"no model", wire.SessionSwitchRequest{ID: id}, wire.ReasonSessionBadRequest},
		{"unknown session", wire.SessionSwitchRequest{ID: "nope", Model: "opus"}, wire.ReasonSessionUnknown},
	} {
		if res, err := h.Client.Sessions.Switch(ctx, tc.req); err != nil || !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: switch = %+v, %v; want refused %s", tc.name, res, err, tc.reason)
		}
	}

	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res, err := h.Client.Sessions.Switch(ctx, req); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Errorf("switch working = %+v, %v; want refused %s", res, err, wire.ReasonSessionNotIdle)
	}

	stopFakeSession(ctx, t, h, evs, id)
	if res, err := h.Client.Sessions.Switch(ctx, req); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotLive {
		t.Errorf("switch exited = %+v, %v; want refused %s", res, err, wire.ReasonSessionNotLive)
	}
	if m := readMeta(t, h, id); m["model"] != before["model"] || m["effort"] != before["effort"] || m["switched_by_hand"] != nil {
		t.Errorf("meta after refusals = %v, want the model and effort of %v", m, before)
	}
}

// A harness without a chat reader is neither Claude nor Codex.
func TestSwitchSession_RefusesUnsupportedHarness(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSession(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if res, err := h.Client.Sessions.Switch(ctx, wire.SessionSwitchRequest{ID: id, Model: "opus"}); err != nil || !res.Refused || res.Reason != wire.ReasonSessionUnknownHarness {
		t.Errorf("switch = %+v, %v; want refused %s", res, err, wire.ReasonSessionUnknownHarness)
	}
}

// The web variant shares the interrupt route's token and Queue checks, which
// run before the switch does anything.
func TestSwitchSession_WebNeedsTheStateToken(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, _ := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, state, want string }{
		{"no token", "", wire.ReasonStale},
		{"stale token", "stale", wire.ReasonStale},
		{"fresh token", sessionToken(ctx, t, h, design), wire.ReasonSessionBadRequest},
	} {
		code, body := webSessionWrite(t, base, cookie, "switch", design, tc.state)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
			t.Errorf("web switch with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
}

func TestResumeSession_Refusals(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSession(t)
	if res, err := h.Client.Sessions.Resume(ctx, id); err != nil || !res.Refused || res.Reason != wire.ReasonSessionLive {
		t.Errorf("resume live = %+v, %v; want refused %s", res, err, wire.ReasonSessionLive)
	}

	noResume, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind)})
	if err != nil || noResume.Session == nil {
		t.Fatalf("create = %+v, %v", noResume, err)
	}
	stopFakeSession(ctx, t, h, evs, noResume.Session.ID)
	if res, err := h.Client.Sessions.Resume(ctx, noResume.Session.ID); err != nil || !res.Refused || res.Reason != wire.ReasonSessionNoResume {
		t.Errorf("resume without harness resume = %+v, %v; want refused %s", res, err, wire.ReasonSessionNoResume)
	}

	stopFakeSession(ctx, t, h, evs, id)
	dir, _ := readMeta(t, h, id)["dir"].(string)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Resume(ctx, id)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionDirGone || res.Message != "dir is gone" {
		t.Errorf("resume with dir gone = %+v, %v; want refused %s", res, err, wire.ReasonSessionDirGone)
	}
}
