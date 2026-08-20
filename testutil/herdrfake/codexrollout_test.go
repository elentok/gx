package herdrfake

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

func TestCodexRolloutStartCreatesStableProductionReadableSession(t *testing.T) {
	home := t.TempDir()
	codexHome := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codexHome)

	state := NewState(t)
	resetAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	rollout := RegisterCodexRollout(t, state, CodexRolloutOptions{
		Cwd:       cwd,
		PaneID:    "pane-codex",
		AgentID:   "agent-codex",
		AgentName: "codex",
		InitialUsage: CodexUsage{
			ContextTokens: 180_001,
			TotalTokens:   240_000,
			Primary:       CodexQuota{UsedPercent: 100, ResetsAt: resetAt},
			Secondary:     CodexQuota{UsedPercent: 45, ResetsAt: resetAt.Add(24 * time.Hour)},
		},
	})
	StartState(t, state)

	start := func() herdr.Agent {
		t.Helper()
		agent, err := herdr.AgentStart(herdr.AgentStartOptions{
			Name: "codex", Kind: "codex", Pane: "pane-codex",
		})
		if err != nil {
			t.Fatalf("AgentStart: %v", err)
		}
		return agent
	}
	first := start()
	second := start()
	if first.AgentSession == "" || first.AgentSession != second.AgentSession || first.AgentSession != rollout.SessionID() {
		t.Fatalf("start session ids = %q, %q, model %q; want one stable native id", first.AgentSession, second.AgentSession, rollout.SessionID())
	}
	if first.AgentStatus != "idle" || first.PaneID != "pane-codex" {
		t.Fatalf("started agent = %+v, want idle agent in configured pane", first)
	}

	contextTokens, ok, err := codexsession.LastContextTokens(cwd, first.AgentSession)
	if err != nil || !ok || contextTokens != 180_001 {
		t.Fatalf("LastContextTokens = %d, %v, %v; want production-readable initial usage", contextTokens, ok, err)
	}
	stats, ok, err := codexsession.ReadStats(cwd, first.AgentSession)
	if err != nil || !ok || stats.TotalTokens != 240_000 || stats.PeakContext != 180_001 {
		t.Fatalf("ReadStats = %+v, %v, %v; want authentic total and context usage", stats, ok, err)
	}
	limit, ok, err := codexsession.LastRateLimit(cwd, first.AgentSession)
	if err != nil || !ok || limit.Quota != "primary" || !limit.ResetAt.Equal(resetAt) {
		t.Fatalf("LastRateLimit = %+v, %v, %v; want exhausted primary quota", limit, ok, err)
	}

	rel, err := filepath.Rel(codexHome, rollout.Path())
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		t.Fatalf("rollout path %q is not beneath CODEX_HOME %q", rollout.Path(), codexHome)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("default Codex home was touched: %v", err)
	}
}

func TestCodexRolloutCommandsDriveDistinctPersistentTransitions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	cwd := t.TempDir()
	state := NewState(t)
	resetAt := codexRolloutEpoch.Add(48 * time.Hour)
	rollout := RegisterCodexRollout(t, state, CodexRolloutOptions{
		Cwd: cwd, PaneID: "pane-codex", AgentID: "agent-codex", AgentName: "codex",
		InitialUsage:   CodexUsage{ContextTokens: 180_001, TotalTokens: 240_000},
		CompactedUsage: CodexUsage{ContextTokens: 70_000, TotalTokens: 250_000},
		FinalUsage: CodexUsage{
			ContextTokens: 75_000, TotalTokens: 260_000,
			Secondary: CodexQuota{UsedPercent: 100, ResetsAt: resetAt},
		},
	})
	StartState(t, state)

	started, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "codex", Kind: "codex", Pane: "pane-codex"})
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	prompt := func(text string) herdr.Agent {
		t.Helper()
		agent, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: started.PaneID, Text: text})
		if err != nil {
			t.Fatalf("AgentPrompt(%q): %v", text, err)
		}
		return agent
	}
	wait := func(until string, timeout int) (herdr.Agent, error) {
		t.Helper()
		return herdr.AgentWait(herdr.AgentWaitOptions{Target: started.PaneID, Until: []string{until}, TimeoutMs: timeout})
	}

	if agent := prompt("$implement ticket.md"); agent.AgentStatus != "working" {
		t.Fatalf("implement prompt status = %q, want working", agent.AgentStatus)
	}
	if _, err := wait("idle", 1); err == nil {
		t.Fatal("running wait succeeded, want modeled timeout")
	}
	if err := herdr.AgentSendKeys(started.PaneID, "ctrl+c"); err != nil {
		t.Fatalf("AgentSendKeys: %v", err)
	}
	if agent, err := wait("idle", 0); err != nil || agent.AgentStatus != "idle" {
		t.Fatalf("post-interrupt wait = %+v, %v; want idle, not blocked", agent, err)
	}
	if agent := prompt("/compact"); agent.AgentStatus != "blocked" {
		t.Fatalf("compact confirmation status = %q, want blocked", agent.AgentStatus)
	}
	if agent, err := wait("working", 0); err != nil || agent.AgentStatus != "working" {
		t.Fatalf("compact continuation wait = %+v, %v; want working", agent, err)
	}
	if agent, err := wait("idle", 0); err != nil || agent.AgentStatus != "idle" {
		t.Fatalf("compact final wait = %+v, %v; want idle", agent, err)
	}
	if tokens, ok, err := codexsession.LastContextTokens(cwd, rollout.SessionID()); err != nil || !ok || tokens != 70_000 {
		t.Fatalf("post-compact LastContextTokens = %d, %v, %v; want compacted usage", tokens, ok, err)
	}
	if agent := prompt("please finish up quickly"); agent.AgentStatus != "working" {
		t.Fatalf("finish-up continuation status = %q, want working", agent.AgentStatus)
	}
	if _, err := wait("idle", 1); err == nil {
		t.Fatal("continuation wait succeeded, want modeled timeout before final")
	}
	if agent, err := wait("idle", 0); err != nil || agent.AgentStatus != "idle" {
		t.Fatalf("final wait = %+v, %v; want idle", agent, err)
	}
	if limit, ok, err := codexsession.LastRateLimit(cwd, rollout.SessionID()); err != nil || !ok || limit.Quota != "secondary" || !limit.ResetAt.Equal(resetAt) {
		t.Fatalf("final LastRateLimit = %+v, %v, %v; want secondary quota transition", limit, ok, err)
	}

	wantArgv := [][]string{
		{"agent", "start", "codex", "--kind", "codex", "--pane", "pane-codex"},
		{"agent", "prompt", "pane-codex", "$implement ticket.md"},
		{"agent", "wait", "pane-codex", "--until", "idle", "--timeout", "1"},
		{"agent", "send-keys", "pane-codex", "ctrl+c"},
		{"agent", "wait", "pane-codex", "--until", "idle"},
		{"agent", "prompt", "pane-codex", "/compact"},
		{"agent", "wait", "pane-codex", "--until", "working"},
		{"agent", "wait", "pane-codex", "--until", "idle"},
		{"agent", "prompt", "pane-codex", "please finish up quickly"},
		{"agent", "wait", "pane-codex", "--until", "idle", "--timeout", "1"},
		{"agent", "wait", "pane-codex", "--until", "idle"},
	}
	trace := state.Trace()
	if len(trace) != len(wantArgv) {
		t.Fatalf("trace length = %d, want %d: %+v", len(trace), len(wantArgv), trace)
	}
	for i, entry := range trace {
		if entry.Seq != uint64(i+1) || !reflect.DeepEqual(entry.Argv, wantArgv[i]) {
			t.Fatalf("trace entry %d = seq %d argv %v, want seq %d argv %v", i, entry.Seq, entry.Argv, i+1, wantArgv[i])
		}
		if entry.Identities.AgentID != "agent-codex" || entry.Identities.SessionID != rollout.SessionID() {
			t.Fatalf("trace entry %d identities = %+v, want Codex agent and session", i, entry.Identities)
		}
	}
	if !strings.Contains(trace[3].After, "Status:idle") || !strings.Contains(trace[6].After, "Status:working") || !strings.Contains(trace[10].After, "Status:idle") {
		t.Fatalf("trace snapshots do not expose post-interrupt idle, continuation, and final states: %+v", trace)
	}
	stats, ok, err := codexsession.ReadStats(cwd, rollout.SessionID())
	if err != nil || !ok || stats.TotalTokens != 260_000 || stats.PeakContext != 180_001 || !stats.Start.Equal(codexRolloutEpoch) || !stats.End.Equal(codexRolloutEpoch.Add(3*time.Millisecond)) {
		t.Fatalf("final ReadStats = %+v, %v, %v; want deterministic complete rollout", stats, ok, err)
	}
}

// TestCodexRolloutCtrlCInterruptLeavesPaneIdleNotBlocked locks the real
// contract from ticket 03: a single ctrl+c on a working Codex pane lands in
// idle, not blocked, and a following "/compact" is accepted even with the
// agent_blocked guard switched on. Before this ticket the fake modeled ctrl+c
// as landing in a blocked phase no real Codex pane produces, which made this
// path look like it needed a blocked-pane bypass it never actually needed.
func TestCodexRolloutCtrlCInterruptLeavesPaneIdleNotBlocked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	state := NewState(t)
	RegisterCodexRollout(t, state, CodexRolloutOptions{
		Cwd: t.TempDir(), PaneID: "pane-codex", AgentID: "agent-codex", AgentName: "codex",
		RejectPromptWhenBlocked: true,
	})
	StartState(t, state)

	started, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "codex", Kind: "codex", Pane: "pane-codex"})
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: started.PaneID, Text: "$implement ticket.md"}); err != nil {
		t.Fatalf("AgentPrompt: %v", err)
	}
	if err := herdr.AgentSendKeys(started.PaneID, "ctrl+c"); err != nil {
		t.Fatalf("AgentSendKeys: %v", err)
	}
	if agent, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: started.PaneID, Until: []string{"idle"}}); err != nil || agent.AgentStatus != "idle" {
		t.Fatalf("post-interrupt wait = %+v, %v; want idle", agent, err)
	}

	agent, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: started.PaneID, Text: "/compact"})
	if err != nil {
		t.Fatalf("AgentPrompt(/compact) into post-interrupt pane = %v, want accepted", err)
	}
	if agent.AgentStatus != "blocked" {
		t.Fatalf("compact confirmation status = %q, want blocked", agent.AgentStatus)
	}
}

// TestCodexRolloutRejectPromptWhenBlocked_SwitchOnRejectsWithAgentBlockedError
// covers Codex's one legitimate blocked state: waiting on compact
// confirmation after "/compact" was submitted (see the prior test). That is
// where the agent_blocked guard actually applies.
func TestCodexRolloutRejectPromptWhenBlocked_SwitchOnRejectsWithAgentBlockedError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	state := NewState(t)
	RegisterCodexRollout(t, state, CodexRolloutOptions{
		Cwd: t.TempDir(), PaneID: "pane-codex", AgentID: "agent-codex", AgentName: "codex",
		RejectPromptWhenBlocked: true,
	})
	StartState(t, state)

	started, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "codex", Kind: "codex", Pane: "pane-codex"})
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: started.PaneID, Text: "/compact"}); err != nil {
		t.Fatalf("AgentPrompt(/compact): %v", err)
	}

	_, err = herdr.AgentPrompt(herdr.AgentPromptOptions{Target: started.PaneID, Text: "hello"})
	if err == nil {
		t.Fatal("AgentPrompt into a blocked pane succeeded, want an AgentBlockedError")
	}
	var blocked *herdr.AgentBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("AgentPrompt() error = %v, want *herdr.AgentBlockedError (fake and production classifier must agree)", err)
	}
}
