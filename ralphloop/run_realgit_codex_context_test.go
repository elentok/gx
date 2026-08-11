package ralphloop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

// TestRun_ProductionRealGit_CodexContextRecoveryLandsAndCleansUp drives
// ticket 27's full Codex context-recovery E2E: one real-Git ticket through
// production Run, with a real Codex rollout (ticket 26's herdrfake
// infrastructure, read back through the real codexsession package rather
// than a hand-rolled JSONL fixture as in ticket 15's test) proactively
// breaching --smart-zone, recovering through blocked compact confirmation
// and finish-up, then landing with full trailers, metadata, and cleanup.
// The fake agent's only side effect — committing the ticket's one file — is
// triggered from this test's own "tab create" handler (not from
// herdrfake.RegisterCodexRollout's opaque prompt state machine, which has no
// hook for it), which runs synchronously after runIteration's real
// AddWorktree, so the commit is guaranteed to exist before any wait/finish
// polling begins.
func TestRun_ProductionRealGit_CodexContextRecoveryLandsAndCleansUp(t *testing.T) {
	const (
		epicName  = "epic"
		smartZone = 150000
	)
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-context.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Context recovery\n",
	})

	t.Setenv("HOME", t.TempDir())
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	cwd := iterationWorktreePath(wtDir, epicName, "01")

	s := herdrfake.NewState(t)
	s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
	})
	var commitErr error
	closedTabs := 0
	s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if err := commitIterationWork(cwd, "01"); err != nil {
			commitErr = err
			return nil, herdrfake.Identities{}, err
		}
		return map[string]any{
			"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
			"root_pane": map[string]any{"pane_id": "pane-01"},
		}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
	})
	s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closedTabs++
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("agent", "read", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})

	rollout := herdrfake.RegisterCodexRollout(t, s, herdrfake.CodexRolloutOptions{
		Cwd: cwd, WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01",
		AgentID: "agent-01", AgentName: iterLabel(epicName, "01"),
		InitialUsage:   herdrfake.CodexUsage{ContextTokens: smartZone + 1, TotalTokens: smartZone + 5_000},
		CompactedUsage: herdrfake.CodexUsage{ContextTokens: smartZone / 2, TotalTokens: smartZone + 20_000},
		FinalUsage:     herdrfake.CodexUsage{ContextTokens: smartZone / 2, TotalTokens: smartZone + 25_000},
	})
	herdrfake.StartState(t, s)

	deps := testDeps()
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	sink := &compactRecoverySink{}
	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, deps, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if commitErr != nil {
		t.Fatalf("commitIterationWork: %v", commitErr)
	}

	// Acceptance criterion: the session rollout and Herdr identity match the
	// real iteration cwd — verified through the same production reader
	// (codexsession.VerifyIdentity) reattachment uses, not just by
	// construction of CodexRolloutOptions.
	if ok, err := codexsession.VerifyIdentity(cwd, rollout.SessionID()); err != nil || !ok {
		t.Errorf("VerifyIdentity(%s, %s) = %v, %v; want the rollout's cwd to match the real iteration worktree", cwd, rollout.SessionID(), ok, err)
	}
	stats, ok, err := codexsession.ReadStats(cwd, rollout.SessionID())
	if err != nil || !ok || stats.PeakContext != smartZone+1 || stats.TotalTokens != smartZone+25_000 {
		t.Errorf("ReadStats = %+v, %v, %v; want peak context %d and final total %d read back through the real cwd", stats, ok, err, smartZone+1, smartZone+25_000)
	}

	// Acceptance criterion: one high-context event causes exactly one Ctrl-C
	// and one compact command, and the blocked confirmation completes before
	// finish-up.
	var ctrlCCalls, compactCalls, finishUpCalls int
	var ctrlCIndex, compactIndex, finishUpIndex = -1, -1, -1
	for i, entry := range s.Trace() {
		if len(entry.Argv) < 2 || entry.Argv[0] != "agent" {
			continue
		}
		switch entry.Argv[1] {
		case "send-keys":
			for _, key := range entry.Argv[2:] {
				if key == "ctrl+c" {
					ctrlCCalls++
					ctrlCIndex = i
				}
			}
		case "prompt":
			if len(entry.Argv) < 4 {
				continue
			}
			switch {
			case entry.Argv[3] == "/compact":
				compactCalls++
				compactIndex = i
			case strings.Contains(entry.Argv[3], "please finish up quickly"):
				finishUpCalls++
				finishUpIndex = i
			}
		}
	}
	if ctrlCCalls != 1 || compactCalls != 1 || finishUpCalls != 1 {
		t.Errorf("recovery calls = (ctrl-c:%d compact:%d finish-up:%d), want exactly one each", ctrlCCalls, compactCalls, finishUpCalls)
	}
	if ctrlCIndex == -1 || compactIndex == -1 || finishUpIndex == -1 || !(ctrlCIndex < compactIndex && compactIndex < finishUpIndex) {
		t.Errorf("recovery command order = ctrl-c:%d compact:%d finish-up:%d, want ctrl-c before compact before finish-up", ctrlCIndex, compactIndex, finishUpIndex)
	}
	sink.mu.Lock()
	phases := append([]string{}, sink.phases...)
	sink.mu.Unlock()
	if !slices.Equal(phases, []string{"compact-started", "finishing-up", "recovered"}) {
		t.Errorf("compact phases = %v, want compact-started, finishing-up, recovered", phases)
	}

	// Acceptance criterion: the implementation commit lands with all three
	// Ralph-loop trailers.
	featurePath := filepath.Join(wtDir, epicName)
	trailers, err := git.TrailerMap(featurePath, "HEAD", ticketTrailerKey)
	if err != nil {
		t.Fatalf("TrailerMap: %v", err)
	}
	sha := trailers[ticketTrailerValue(epicName, "01")]
	if sha == "" {
		t.Fatalf("landed trailers = %v, want a landed commit for ticket 01", trailers)
	}
	show := exec.Command("git", "show", "-s", "--format=%B", sha)
	show.Dir = featurePath
	message, err := show.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", sha, err)
	}
	for _, key := range []string{ticketTrailerKey, tokensTrailerKey, elapsedTrailerKey} {
		var values []string
		for _, line := range strings.Split(string(message), "\n") {
			if value, ok := strings.CutPrefix(line, key+": "); ok {
				values = append(values, value)
			}
		}
		if len(values) != 1 {
			t.Errorf("commit %s trailer %s = %v, want exactly one", sha, key, values)
		}
	}

	// Acceptance criterion: ticket frontmatter and lifecycle events are
	// correct, with compactions omitted (Codex has no reliable persisted
	// compaction-count signal).
	ticketPath := filepath.Join(scratchDir, epicName, "issues", "01-context.md")
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket: %v", err)
	}
	frontmatter, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if err := schema.Validate(frontmatter); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if fmt.Sprint(frontmatter.Status) != "done" {
		t.Errorf("ticket status = %v, want done", frontmatter.Status)
	}
	if strings.Contains(string(raw), "compactions:") {
		t.Errorf("completed Codex ticket unexpectedly persisted compactions: %s", raw)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	wantEventOrder := []string{eventPausedSmartZone, eventResumed, eventIterationFinished, eventCherryPicked}
	var gotEventOrder []string
	for _, event := range events {
		if event.Type == eventNeedsAnswer || event.Type == eventNeedsRepair || event.Type == eventSmartZoneRecoveryFailed {
			t.Errorf("unexpected recovery residue event: %+v", event)
		}
		if slices.Contains(wantEventOrder, event.Type) {
			gotEventOrder = append(gotEventOrder, event.Type)
		}
		if event.Type == eventCherryPicked && event.SHA != sha {
			t.Errorf("cherry-picked SHA = %q, want landed SHA %q", event.SHA, sha)
		}
	}
	if !slices.Equal(gotEventOrder, wantEventOrder) {
		t.Errorf("recovery event order = %v, want %v", gotEventOrder, wantEventOrder)
	}

	// Acceptance criterion: ticket, worktree, tab, and branch finish cleanly.
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Errorf("iteration worktree %s still exists, want removed", cwd)
	}
	branch := iterBranch(epicName, "01")
	if _, err := git.RevParse(featurePath, branch); err == nil {
		t.Errorf("iteration branch %s still exists, want deleted", branch)
	}
	if closedTabs != 1 {
		t.Errorf("closed tabs = %d, want exactly one", closedTabs)
	}
	if _, err := os.Stat(featurePath); err != nil {
		t.Errorf("feature worktree removed: %v", err)
	}
	if _, err := git.RevParse(featurePath, epicName); err != nil {
		t.Errorf("feature branch removed: %v", err)
	}
}

// codexNativeContextFixture wires up the shared repo/ticket/session scaffolding
// for the two ticket-31 scenarios below: a Codex pane reporting a native
// context-window exhaustion banner (the process-boundary text `herdr agent
// read` surfaces) while the session's own rollout stays well under
// --smart-zone for the whole run. That low occupancy is the point: recovery
// here must come from classifying the pane text itself through the same
// production Herdr/session wiring as any other Codex iteration, never from a
// fresh high-occupancy rollout event (see
// TestRun_ProductionRealGit_CodexCompactsThenCompletes for the occupancy-driven
// counterpart).
func codexNativeContextFixture(t *testing.T) (repoDir, scratchDir, ticketPath, cwd string) {
	t.Helper()
	const epicName = "epic"
	const smartZone = 150000
	const sessionID = "codex-session-31"

	repoDir = testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	scratchDir = writeEpic(t, epicName, map[string]string{
		"01-native.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Native context recovery\n",
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	cwd = iterationWorktreePath(wtDir, epicName, "01")
	sessionPath := filepath.Join(home, ".codex", "sessions", "2026", "08", "04", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0755); err != nil {
		t.Fatalf("MkdirAll Codex session: %v", err)
	}
	lowSession := fmt.Sprintf(
		"{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"cwd\":%q}}\n"+
			"{\"type\":\"event_msg\",\"payload\":{\"type\":\"token_count\",\"info\":{\"last_token_usage\":{\"input_tokens\":%d}}}}\n",
		sessionID, cwd, smartZone/4,
	)
	if err := os.WriteFile(sessionPath, []byte(lowSession), 0644); err != nil {
		t.Fatalf("WriteFile Codex session: %v", err)
	}
	ticketPath = filepath.Join(scratchDir, epicName, "issues", "01-native.md")
	return repoDir, scratchDir, ticketPath, cwd
}

// TestRun_ProductionRealGit_CodexNativeContextExhaustionRecovers exercises
// ticket 31's successful-recovery path: exactly one classification (ctrl+c,
// /compact, finish-up) fires off the native-exhaustion banner, the agent
// finishes and commits, and the ticket lands normally.
func TestRun_ProductionRealGit_CodexNativeContextExhaustionRecovers(t *testing.T) {
	const (
		epicName  = "epic"
		smartZone = 150000
		sessionID = "codex-session-31"
		evidence  = "stream disconnected before completion: your input exceeds the context window of this model"
	)
	repoDir, scratchDir, ticketPath, cwd := codexNativeContextFixture(t)

	s := herdrfake.NewState(t)
	phase := "starting"
	ctrlCCalls, compactCalls, evidenceReads := 0, 0, 0
	var recoveryPrompts []string

	s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
	})
	s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{
			"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
			"root_pane": map[string]any{"pane_id": "pane-01"},
		}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
	})
	s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID},
		}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		text := argv[3]
		switch {
		case strings.HasPrefix(text, "/implement "), strings.HasPrefix(text, "$implement "):
			phase = "implementing"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case text == "/compact":
			compactCalls++
			recoveryPrompts = append(recoveryPrompts, "/compact")
			phase = "compact-blocked"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "blocked", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case strings.Contains(text, "please finish up quickly"):
			recoveryPrompts = append(recoveryPrompts, "finish-up")
			if err := commitIterationWork(cwd, "01"); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			phase = "done"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected prompt %q", text)
		}
	})
	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		switch phase {
		case "starting":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "implementing":
			// The pane never settles into a finish state on its own here:
			// the loop must classify the native-exhaustion banner off
			// ReadPaneRecent during this very poll timeout, not wait for the
			// pane to go idle first.
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		case "compact-blocked":
			phase = "compacting"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "compacting":
			phase = "compacted"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "done":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected wait in phase %q", phase)
		}
	})
	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		for _, key := range argv[3:] {
			if key == "ctrl+c" {
				ctrlCCalls++
			}
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "read", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if phase == "implementing" && evidenceReads == 0 {
			evidenceReads++
			return evidence, herdrfake.Identities{}, nil
		}
		return "", herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, s)

	deps := testDeps()
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	sink := &compactRecoverySink{}
	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, deps, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if evidenceReads != 1 {
		t.Errorf("evidenceReads = %d, want exactly 1", evidenceReads)
	}
	if ctrlCCalls != 1 || compactCalls != 1 {
		t.Errorf("recovery calls = ctrl+c:%d compact:%d, want exactly one each", ctrlCCalls, compactCalls)
	}
	if !slices.Equal(recoveryPrompts, []string{"/compact", "finish-up"}) {
		t.Errorf("recovery prompts = %v, want compact then finish-up", recoveryPrompts)
	}
	sink.mu.Lock()
	phases := append([]string{}, sink.phases...)
	sink.mu.Unlock()
	if !slices.Equal(phases, []string{"compact-started", "finishing-up", "recovered"}) {
		t.Errorf("compact phases = %v, want compact-started, finishing-up, recovered", phases)
	}

	rawTicket, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile completed ticket: %v", err)
	}
	if !strings.Contains(string(rawTicket), "status: done") {
		t.Errorf("completed ticket = %s, want done", rawTicket)
	}
	if strings.Contains(string(rawTicket), "status: needs-answer") || strings.Contains(string(rawTicket), "status: needs-repair") {
		t.Errorf("completed ticket = %s, want neither needs-answer nor needs-repair", rawTicket)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var pausedReason string
	wantEventOrder := []string{eventPausedSmartZone, eventResumed, eventIterationFinished, eventCherryPicked}
	var gotEventOrder []string
	for _, event := range events {
		switch event.Type {
		case eventNeedsAnswer, eventNeedsRepair, eventSmartZoneRecoveryFailed:
			t.Errorf("unexpected recovery residue event: %+v", event)
		case eventPausedSmartZone:
			pausedReason = event.Reason
			gotEventOrder = append(gotEventOrder, event.Type)
		case eventResumed, eventIterationFinished, eventCherryPicked:
			gotEventOrder = append(gotEventOrder, event.Type)
		}
	}
	if !slices.Equal(gotEventOrder, wantEventOrder) {
		t.Errorf("recovery event order = %v, want %v", gotEventOrder, wantEventOrder)
	}
	if !strings.Contains(pausedReason, "Codex context exhaustion detected") || !strings.Contains(pausedReason, evidence) {
		t.Errorf("paused-smart-zone reason = %q, want it to name the native context exhaustion and its evidence", pausedReason)
	}
	if strings.Contains(pausedReason, "context occupancy") {
		t.Errorf("paused-smart-zone reason = %q, want the pane-text classification reason, not the occupancy-breach one", pausedReason)
	}
}

// TestRun_ProductionRealGit_CodexNativeContextExhaustionRecoveryFails
// exercises ticket 31's unsuccessful-recovery path: the same native
// exhaustion banner is classified and the pane is interrupted, but the
// /compact prompt itself never lands. That must turn into a durable,
// actionable needs-repair failure — not a false "done" (no commit ever
// lands) and not a generic zero-commit needs-answer that would lose the
// exhaustion reason — and it must leave the iteration's worktree/branch/tab
// in place for a human to inspect.
func TestRun_ProductionRealGit_CodexNativeContextExhaustionRecoveryFails(t *testing.T) {
	const (
		epicName  = "epic"
		smartZone = 150000
		sessionID = "codex-session-31"
		// No literal quotes: the herdrfake State JSON-envelopes every command
		// result (including pane text), so a quote-bearing evidence string
		// like the structured context_length_exceeded error code would come
		// back escaped and silently fail detectCodexContextExhaustion's
		// literal Contains checks.
		evidence = "stream disconnected before completion: your input exceeds the context window of this model"
	)
	repoDir, scratchDir, ticketPath, _ := codexNativeContextFixture(t)
	wtDir := testWorktreeDir(t, repoDir)

	s := herdrfake.NewState(t)
	phase := "starting"
	ctrlCCalls, compactCalls, evidenceReads := 0, 0, 0
	var recoveryPrompts []string

	s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
	})
	s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{
			"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
			"root_pane": map[string]any{"pane_id": "pane-01"},
		}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
	})
	closedTabs := 0
	s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closedTabs++
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID},
		}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		text := argv[3]
		switch {
		case strings.HasPrefix(text, "/implement "), strings.HasPrefix(text, "$implement "):
			phase = "implementing"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case text == "/compact":
			compactCalls++
			recoveryPrompts = append(recoveryPrompts, "/compact")
			return nil, herdrfake.Identities{}, fmt.Errorf("compact never landed")
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected prompt %q", text)
		}
	})
	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		switch phase {
		case "starting":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "implementing":
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected wait in phase %q", phase)
		}
	})
	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		for _, key := range argv[3:] {
			if key == "ctrl+c" {
				ctrlCCalls++
			}
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "read", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if phase == "implementing" && evidenceReads == 0 {
			evidenceReads++
			return evidence, herdrfake.Identities{}, nil
		}
		return "", herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, s)

	deps := testDeps()
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.Sleep = func(time.Duration) {}
	deps.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	sink := &needsRepairSink{}
	// The failed recovery leaves the epic's only ticket needs-repair, so
	// the run parks on it.
	runUntilParked(t, RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, deps, sink)

	if evidenceReads != 1 {
		t.Errorf("evidenceReads = %d, want exactly 1", evidenceReads)
	}
	if ctrlCCalls != 1 || compactCalls != 1 {
		t.Errorf("recovery calls = ctrl+c:%d compact:%d, want exactly one each", ctrlCCalls, compactCalls)
	}
	if !slices.Equal(recoveryPrompts, []string{"/compact"}) {
		t.Errorf("recovery prompts = %v, want only the failed /compact attempt, no finish-up", recoveryPrompts)
	}

	sink.mu.Lock()
	paused, kind, reason := sink.paused, sink.kind, sink.reason
	sink.mu.Unlock()
	if !paused || kind != PauseNeedsRepair {
		t.Fatalf("IterationPaused = (paused:%v kind:%q), want (true, %q)", paused, kind, PauseNeedsRepair)
	}
	if !strings.Contains(reason, "recovery failed") || !strings.Contains(reason, evidence) {
		t.Errorf("needs-repair reason = %q, want it to name the failed recovery and its evidence", reason)
	}

	rawTicket, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket: %v", err)
	}
	if !strings.Contains(string(rawTicket), "status: needs-repair") {
		t.Errorf("ticket = %s, want needs-repair", rawTicket)
	}
	if strings.Contains(string(rawTicket), "status: done") || strings.Contains(string(rawTicket), "status: needs-answer") {
		t.Errorf("ticket = %s, want neither done nor needs-answer (would drop the exhaustion reason)", rawTicket)
	}
	if !strings.Contains(string(rawTicket), "## Needs Repair") || !strings.Contains(string(rawTicket), evidence) {
		t.Errorf("ticket = %s, want a durable Needs Repair note naming the exhaustion evidence", rawTicket)
	}

	events, ok, err := readEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var sawPaused, sawRecoveryFailed, sawFinished, sawCherryPicked, sawNeedsAnswer bool
	for _, event := range events {
		switch event.Type {
		case eventPausedSmartZone:
			sawPaused = true
			if !strings.Contains(event.Reason, "Codex context exhaustion detected") {
				t.Errorf("paused-smart-zone reason = %q, want it to name the native context exhaustion", event.Reason)
			}
		case eventSmartZoneRecoveryFailed:
			sawRecoveryFailed = true
		case eventIterationFinished:
			sawFinished = true
		case eventCherryPicked:
			sawCherryPicked = true
		case eventNeedsAnswer:
			sawNeedsAnswer = true
		case eventResumed:
			t.Error("unexpected resumed event: recovery never succeeded")
		}
	}
	if !sawPaused || !sawRecoveryFailed {
		t.Errorf("event log missing classification/recovery-failure trail: paused=%v recoveryFailed=%v", sawPaused, sawRecoveryFailed)
	}
	if sawFinished || sawCherryPicked {
		t.Error("iteration must not appear finished/cherry-picked: no commit ever landed")
	}
	if sawNeedsAnswer {
		t.Error("must not fall back to generic needs-answer: that would lose the exhaustion reason")
	}

	path := iterationWorktreePath(wtDir, epicName, "01")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("iteration worktree removed (%v), want retained for inspection", err)
	}
	featurePath := filepath.Join(wtDir, epicName)
	branch := iterBranch(epicName, "01")
	if _, err := git.RevParse(featurePath, branch); err != nil {
		t.Errorf("iteration branch removed (%v), want retained for inspection", err)
	}
	if closedTabs != 0 {
		t.Errorf("closedTabs = %d, want 0: the iteration tab must stay open for inspection", closedTabs)
	}
}

// needsRepairSink records the single IterationPaused call ticket 31's
// failed-recovery scenario is expected to make.
type needsRepairSink struct {
	noopEventSink
	mu     sync.Mutex
	paused bool
	kind   PauseKind
	reason string
}

func (s *needsRepairSink) IterationPaused(label string, kind PauseKind, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
	s.kind = kind
	s.reason = reason
}
