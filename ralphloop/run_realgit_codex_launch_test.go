package ralphloop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets/schema"
)

// authenticatedCodexLoginScript is a fake `codex` binary that answers `codex
// login status` as already logged in and fails any other invocation —
// reused by every launch-failure scenario whose codex step must pass so the
// failure under test (a missing/incompatible Herdr integration) is reached.
const authenticatedCodexLoginScript = `if [ "$1 $2" = "login status" ]; then
  echo 'Logged in using ChatGPT'
  exit 0
fi
exit 1
`

// TestRun_ProductionRealGit_CodexLaunchPreflightFailures drives ticket 24's
// preflight through the real process boundary Run uses for a successful
// Codex launch: real exec.LookPath/exec.Command against a controlled PATH
// (a hand-rolled fake `codex` executable, and — only for the case that
// actually reaches it — a fake `herdr` via testutil/herdrfake), rather than
// loop_agent_test.go's function-injected preflightAgentWith. Each case
// asserts ticket 32's shared launch-failure outcomes via assertNoLaunchTrace
// on top of its own distinct, actionable error message.
func TestRun_ProductionRealGit_CodexLaunchPreflightFailures(t *testing.T) {
	// not parallel-safe: the "incompatible herdr integration" case's subtest
	// calls herdrfake.Start, which calls t.Setenv — and Setenv panics if the
	// parent test has called t.Parallel, so this outer test must stay
	// sequential too.
	for _, tc := range []struct {
		name          string
		codexScript   string // "" leaves `codex` absent from PATH entirely
		withHerdrFake bool
		herdrHelp     string // "agent start --help" response, when withHerdrFake
		wantErr       string
	}{
		{
			name:    "missing codex executable",
			wantErr: "codex executable not found in PATH; install Codex or add it to PATH",
		},
		{
			name: "codex not authenticated",
			codexScript: `if [ "$1 $2" = "login status" ]; then
  echo 'Not logged in'
  exit 0
fi
exit 1
`,
			wantErr: "codex is not authenticated; run `codex login`",
		},
		{
			name:        "missing herdr executable",
			codexScript: authenticatedCodexLoginScript,
			wantErr:     "Herdr executable not found in PATH; install or upgrade Herdr with Codex integration",
		},
		{
			name:          "incompatible herdr integration",
			codexScript:   authenticatedCodexLoginScript,
			withHerdrFake: true,
			herdrHelp:     "[possible values: claude, gemini]",
			wantErr:       "installed Herdr does not support Codex agents; upgrade Herdr",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			realGitTimeoutWatchdog(t, realGitTestTimeout)
			const epicName = "epic"
			repoDir := testutil.TempRepo(t)
			scratchDir := writeEpic(t, epicName, map[string]string{
				"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
			})
			home := t.TempDir()
			pathEnv := pathExcluding("codex", "herdr")
			if tc.codexScript != "" {
				codexDir := writeFakeExecutable(t, "codex", tc.codexScript)
				pathEnv = codexDir + string(os.PathListSeparator) + pathExcluding("codex", "herdr")
			}

			herdrCalls := 0
			if tc.withHerdrFake {
				// herdrfake.Start prepends its fake herdr's bin dir to the real
				// process PATH via its own t.Setenv (outside this ticket's
				// scope to change) — recover that bin dir from the PATH delta
				// so it can be folded into pathEnv, which is what deps below
				// actually resolves lookups against.
				origPath := os.Getenv("PATH")
				herdrfake.Start(t, func(argv []string) ([]byte, int) {
					herdrCalls++
					if len(argv) == 3 && argv[0] == "agent" && argv[1] == "start" && argv[2] == "--help" {
						return []byte(tc.herdrHelp), 0
					}
					return herdrfake.CommandError("unexpected herdr command in launch-preflight test: " + strings.Join(argv, " "))
				})
				herdrDir := strings.TrimSuffix(os.Getenv("PATH"), string(os.PathListSeparator)+origPath)
				pathEnv = herdrDir + string(os.PathListSeparator) + pathEnv
			}

			deps := testDepsWithOverrides(DepsOverrides{Home: home, Path: pathEnv})
			deps.Sleep = func(time.Duration) {}

			sink := newRecordingEventSink()
			err := Run(RunOptions{
				EpicName: epicName, Agent: AgentCodex, Skill: "implement",
				ScratchDir: scratchDir, RepoDir: repoDir,
			}, deps, sink)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Run() error = %v, want containing %q", err, tc.wantErr)
			}

			assertNoLaunchTrace(t, repoDir, epicName, scratchDir, "01-first.md", sink)

			// Tab outcome for the one case that actually reaches Herdr: exactly
			// the "agent start --help" probe, no tab/workspace command.
			if tc.withHerdrFake && herdrCalls != 1 {
				t.Errorf("herdr calls = %d, want exactly 1 (the agent start --help probe, no tab ever opened)", herdrCalls)
			}
		})
	}
}

// TestRun_ProductionRealGit_MissingSkillFailsBeforeClaim drives ticket 25's
// missing-implementation-skill check through the real process boundary: a
// real HOME with no ~/.claude/skills/implement/SKILL.md, exercised via
// DefaultDeps().VerifySkill (verifySkill in deps.go) rather than a stubbed
// function, asserting the same shared launch-failure outcomes plus the
// skill's own specific, actionable reason.
func TestRun_ProductionRealGit_MissingSkillFailsBeforeClaim(t *testing.T) {
	t.Parallel()
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	home := t.TempDir()

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.Sleep = func(time.Duration) {}

	sink := newRecordingEventSink()
	err := Run(RunOptions{
		EpicName: epicName, Skill: "implement", ScratchDir: scratchDir, RepoDir: repoDir,
	}, deps, sink)
	wantErr := fmt.Sprintf(`skill "implement" not found at %s`, filepath.Join(home, ".claude", "skills", "implement", "SKILL.md"))
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("Run() error = %v, want containing %q", err, wantErr)
	}

	assertNoLaunchTrace(t, repoDir, epicName, scratchDir, "01-first.md", sink)
}

// TestRun_ProductionRealGit_CodexLaunchFailureAfterClaimNeedsRepair drives
// the remaining half of ticket 32's "later failures leave deliberate durable
// state" acceptance criterion: a real git worktree/branch and a real
// herdrfake.State-backed workspace/tab are created for ticket "01" exactly as
// a successful Codex launch would (see
// TestRun_ProductionRealGit_CodexContextRecoveryLandsAndCleansUp), then
// `herdr agent start` itself fails. Unlike
// TestRun_ProductionRealGit_CodexLaunchPreflightFailures and
// TestRun_ProductionRealGit_MissingSkillFailsBeforeClaim above (both caught
// before the ticket is ever claimed), this failure lands after
// AddWorktree/InstallDeps/TabCreate have already run for real, so the
// durable state they left — worktree, branch, tab — must survive, and the
// ticket must land on status: needs-repair (not needs-answer/done/claimed),
// matching loop_agent_test.go's
// TestRun_CodexLaunchFailureAfterClaimNeedsRepair but through real git +
// herdrfake.State instead of stubbed Deps functions.
func TestRun_ProductionRealGit_CodexLaunchFailureAfterClaimNeedsRepair(t *testing.T) {
	// not parallel-safe: herdrfake.StartState calls t.Setenv for the helper
	// socket path and PATH.
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const epicName = "epic"
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	home := t.TempDir()

	s := herdrfake.NewState(t)
	s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
	})
	tabClosed := 0
	s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{
			"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
			"root_pane": map[string]any{"pane_id": "pane-01"},
		}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
	})
	s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		tabClosed++
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
	})
	agentStartCalls := 0
	s.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		agentStartCalls++
		return nil, herdrfake.Identities{}, fmt.Errorf("Herdr rejected Codex integration")
	})
	herdrfake.StartState(t, s)

	deps := testDepsWithOverrides(DepsOverrides{Home: home})
	deps.PreflightAgent = func(AgentKind) error { return nil }
	deps.VerifySkill = func(AgentKind, string) error { return nil }
	deps.Sleep = func(time.Duration) {}

	sink := newRecordingEventSink()
	// The failed launch leaves the epic's only ticket needs-repair, so the
	// run parks on it.
	runUntilParked(t, RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement",
		ScratchDir: scratchDir, RepoDir: repoDir,
	}, deps, sink)

	if agentStartCalls != 1 {
		t.Errorf("agent start calls = %d, want exactly 1", agentStartCalls)
	}
	if tabClosed != 0 {
		t.Errorf("tab closed = %d, want 0 (launch failure leaves the tab in place)", tabClosed)
	}

	worktreePath := iterationWorktreePath(wtDir, epicName, "01")
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("Stat(%q) = %v, want the iteration worktree left in place after launch failure", worktreePath, err)
	}
	branch := iterBranch(epicName, "01")
	if branchOut, err := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", branch).CombinedOutput(); err != nil {
		t.Errorf("git rev-parse --verify %s: %v\n%s, want the iteration branch left in place", branch, err, branchOut)
	}

	ticketPath := filepath.Join(scratchDir, epicName, "issues", "01-first.md")
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket: %v", err)
	}
	for _, unwanted := range []string{"status: done", "status: needs-answer", "status: claimed"} {
		if strings.Contains(string(raw), unwanted) {
			t.Errorf("ticket after launch failure = %s, must not contain %q", raw, unwanted)
		}
	}
	if !strings.Contains(string(raw), "status: needs-repair") ||
		!strings.Contains(string(raw), "launching codex") ||
		!strings.Contains(string(raw), "Herdr rejected Codex integration") {
		t.Errorf("ticket after launch failure = %s, want durable needs-repair status and launch reason", raw)
	}

	events, ok, err := ReadEvents(scratchDir, epicName)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if ok {
		for _, event := range events {
			switch event.Type {
			case eventIterationStarted, eventIterationFinished, eventCherryPicked, eventNeedsAnswer:
				t.Errorf("unexpected success/finish-family event after launch failure: %+v", event)
			}
		}
	}

	if hasEvent(sink, LiveEventIterationFinished, func(LiveEvent) bool { return true }) ||
		hasEvent(sink, LiveEventTicketNeedsHuman, func(ev LiveEvent) bool { return ev.Status == "needs-answer" }) {
		t.Errorf("events = %+v, must not report successful/generic completion", sink.Events())
	}
}

// TestRun_ProductionRealGit_CodexRestartReattachesAndLandsOnce drives ticket
// 30's restart/reattach E2E: a first Run invocation is frozen mid-iteration
// (ticket claimed, worktree/tab/agent all live, agent mid-turn) by blocking
// its Deps-level AgentWait client-side — never inside a herdrfake.State
// handler, which must never block holding State's shared mutex — then
// abandoned as a leaked goroutine. A second, independent Run invocation
// against the same herdrfake State and real Codex rollout reattaches to that
// still-live iteration (via reconcile's claimed+live-tab path), continues
// polling the same session rather than launching or prompting a fresh one,
// and lands it to completion exactly once.
func TestRun_ProductionRealGit_CodexRestartReattachesAndLandsOnce(t *testing.T) {
	// not parallel-safe: setProcessEnv mutates the process-wide $HOME/
	// $CODEX_HOME env vars, and herdrfake.StartState calls t.Setenv for the
	// helper socket path and PATH.
	realGitTimeoutWatchdog(t, realGitTestTimeout)
	const (
		epicName  = "epic"
		smartZone = 150000
		sessionID = "codex-session-01"
	)
	repoDir := testutil.TempRepo(t)
	wtDir := testWorktreeDir(t, repoDir)
	scratchDir := writeEpic(t, epicName, map[string]string{
		"01-restart.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Restart\n",
	})

	home := t.TempDir()
	codexHome := filepath.Join(home, ".codex")
	// writeLandedMetrics (report_metrics.go) reads sessionID's Codex stats
	// through codexsession.ReadStats directly, with no DepsOverrides hook of
	// its own (it's also used by the standalone `gx ralph-loop report`
	// command, which always wants real env) — so this test keeps real HOME/
	// CODEX_HOME pointed at the fixture too, alongside the DepsOverrides
	// below that cover everything Run() itself reads.
	setProcessEnv(t, "HOME", home)
	setProcessEnv(t, "CODEX_HOME", codexHome)
	cwd := iterationWorktreePath(wtDir, epicName, "01")
	sessionPath := filepath.Join(home, ".codex", "sessions", "2026", "08", "04", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0755); err != nil {
		t.Fatalf("MkdirAll Codex session: %v", err)
	}
	// Low initial usage, well under smartZone: this scenario is about
	// restart/reattach only, not context recovery (ticket 27/31's job), so
	// no compact/quota path should ever trigger.
	initialSession := fmt.Sprintf(
		"{\"type\":\"session_meta\",\"timestamp\":\"2026-08-04T12:00:00Z\",\"payload\":{\"id\":%q,\"cwd\":%q}}\n"+
			"{\"type\":\"event_msg\",\"timestamp\":\"2026-08-04T12:00:01Z\",\"payload\":{\"type\":\"token_count\",\"info\":{\"last_token_usage\":{\"input_tokens\":%d}}}}\n",
		sessionID, cwd, smartZone/10,
	)
	if err := os.WriteFile(sessionPath, []byte(initialSession), 0644); err != nil {
		t.Fatalf("WriteFile Codex session: %v", err)
	}

	ticketPath := filepath.Join(scratchDir, epicName, "issues", "01-restart.md")

	s := herdrfake.NewState(t)
	// phase/implementingWaits/tabOpen/closedTabs/implementPrompts are only
	// ever touched from inside a registered CommandFunc (so implicitly
	// serialized by State's dispatch mutex) or read from the test goroutine
	// after every Run call that could still touch them has returned or is
	// permanently frozen (see freeze below) — no separate mutex needed.
	phase := "starting"
	implementingWaits := 0
	tabOpen := false
	closedTabs := 0
	var implementPrompts []string

	s.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	s.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "ws1"}}, herdrfake.Identities{WorkspaceID: "ws1"}, nil
	})
	s.Register("tab", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		tabOpen = true
		return map[string]any{
			"tab":       map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
			"root_pane": map[string]any{"pane_id": "pane-01"},
		}, herdrfake.Identities{WorkspaceID: "ws1", TabID: "tab-01", PaneID: "pane-01"}, nil
	})
	s.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		closedTabs++
		tabOpen = false
		return nil, herdrfake.Identities{TabID: "tab-01"}, nil
	})
	s.Register("tab", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if !tabOpen {
			return map[string]any{"tabs": []any{}}, herdrfake.Identities{}, nil
		}
		return map[string]any{"tabs": []any{
			map[string]any{"tab_id": "tab-01", "label": iterLabel(epicName, "01"), "workspace_id": "ws1"},
		}}, herdrfake.Identities{}, nil
	})
	s.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID},
		}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	// "agent get" is needed because both reconcile()'s reattach() closure and
	// reattachIteration call d.AgentGet(label) — the compact-recovery
	// template this test follows never needed it since it has no reattach.
	s.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		status := "working"
		if phase != "implementing" {
			status = "idle"
		}
		return map[string]any{"agent": map[string]any{
			"pane_id": "pane-01", "workspace_id": "ws1", "tab_id": "tab-01",
			"agent_status": status, "agent_session": map[string]any{"value": sessionID},
		}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	s.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		text := argv[3]
		if !strings.HasPrefix(text, "$implement ") {
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected prompt %q", text)
		}
		implementPrompts = append(implementPrompts, text)
		phase = "implementing"
		return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	// implementingWaits drives "running -> idle" with no ctrl+c/compact/
	// finish-up self-transition: the first two polls (both from invocation
	// 2, since invocation 1's second AgentWait call never reaches State —
	// see the d1.AgentWait override below) time out, proving invocation 2
	// keeps polling the same reattached session; the third commits the
	// iteration's work and finishes it.
	s.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		switch phase {
		case "starting":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "implementing":
			implementingWaits++
			if implementingWaits < 3 {
				return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
			}
			if err := commitIterationWork(cwd, "01"); err != nil {
				return nil, herdrfake.Identities{}, err
			}
			phase = "done"
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		case "done":
			return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "idle", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
		default:
			return nil, herdrfake.Identities{}, fmt.Errorf("unexpected wait in phase %q", phase)
		}
	})
	s.Register("agent", "send-keys", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{"pane_id": "pane-01", "agent_status": "working", "agent_session": map[string]any{"value": sessionID}}}, herdrfake.Identities{PaneID: "pane-01", SessionID: sessionID}, nil
	})
	// "agent read" is needed because codexQuotaOrContextExhaustion's
	// d.ReadPaneRecent fallback calls it (via herdr's "agent read <pane>");
	// no exhaustion evidence is ever found, so every poll-timeout falls
	// through to the ordinary smart-zone occupancy check.
	s.Register("agent", "read", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return "", herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, s)

	// contextReads records every (cwd, sessionID) pair either invocation's
	// ReadCodexContext observes — direct evidence (acceptance criterion 3)
	// that context observation continues against the one original session
	// across both invocations, rather than being inferred from side effects.
	var contextReadsMu sync.Mutex
	var contextReads []struct{ cwd, sessionID string }
	recordContextRead := func(cwd, sid string) {
		contextReadsMu.Lock()
		defer contextReadsMu.Unlock()
		contextReads = append(contextReads, struct{ cwd, sessionID string }{cwd, sid})
	}

	d1 := testDepsWithOverrides(DepsOverrides{Home: home, CodexHome: codexHome})
	d1.PreflightAgent = func(AgentKind) error { return nil }
	d1.VerifySkill = func(AgentKind, string) error { return nil }
	d1.Sleep = func(time.Duration) {}
	d1.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	realReadContext1 := d1.ReadCodexContext
	d1.ReadCodexContext = func(cwd, sid string) (int, bool, error) {
		recordContextRead(cwd, sid)
		return realReadContext1(cwd, sid)
	}

	// Simulate invocation 1's stop without corrupting reconcile-visible
	// state and without deadlocking herdrfake.State's shared mutex: block at
	// the Deps-level AgentWait wrapper, client side, after the real call has
	// already returned and released State's lock — never inside a
	// State.Register handler, which per its doc comment must never block
	// holding State's mutex. The wrapper's first call (the harmless post-
	// AgentStart "wait until idle") passes through to the real AgentWait;
	// starting on its second call (the first real poll inside waitForFinish,
	// once phase == "implementing") it blocks forever on an unclosed channel
	// receive instead.
	realWait1 := d1.AgentWait
	waitCalls1 := 0
	freeze := make(chan struct{})
	ctx1, cancel1 := context.WithCancel(context.Background())
	t.Cleanup(cancel1)
	d1.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls1++
		if waitCalls1 == 1 {
			return realWait1(opts)
		}
		close(freeze)
		<-ctx1.Done()
		return herdr.Agent{}, ctx1.Err()
	}

	done1 := make(chan error, 1)
	go func() {
		done1 <- Run(RunOptions{
			EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
			RepoDir: repoDir, SmartZone: smartZone, Ctx: ctx1,
		}, d1, noopEventSink{})
	}()
	t.Cleanup(func() {
		cancel1()
		select {
		case <-done1:
		case <-time.After(30 * time.Second):
			t.Error("Run() invocation 1 did not stop after cancellation")
		}
	})

	<-freeze

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile ticket after invocation 1 freeze: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Fatalf("ticket after invocation 1 freeze = %s, want claimed", raw)
	}
	if !tabOpen {
		t.Fatal("iteration tab not live after invocation 1 freeze, want it left open for reattach")
	}
	if phase != "implementing" {
		t.Fatalf("phase after invocation 1 freeze = %q, want implementing", phase)
	}

	d2 := testDepsWithOverrides(DepsOverrides{Home: home, CodexHome: codexHome})
	d2.PreflightAgent = func(AgentKind) error { return nil }
	d2.VerifySkill = func(AgentKind, string) error { return nil }
	d2.Sleep = func(time.Duration) {}
	d2.Now = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }
	realReadContext2 := d2.ReadCodexContext
	d2.ReadCodexContext = func(cwd, sid string) (int, bool, error) {
		recordContextRead(cwd, sid)
		return realReadContext2(cwd, sid)
	}

	if err := Run(RunOptions{
		EpicName: epicName, Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir,
		RepoDir: repoDir, SmartZone: smartZone,
	}, d2, noopEventSink{}); err != nil {
		t.Fatalf("Run() invocation 2 error = %v", err)
	}

	// Acceptance criterion: exactly one implementation prompt across both
	// invocations — reattachIteration never replays the initial prompt.
	if len(implementPrompts) != 1 {
		t.Errorf("implement prompts = %v, want exactly one across both invocations", implementPrompts)
	}

	// Acceptance criterion: context observation continued against the one
	// original session throughout, in both invocations.
	contextReadsMu.Lock()
	gotContextReads := append([]struct{ cwd, sessionID string }{}, contextReads...)
	contextReadsMu.Unlock()
	if len(gotContextReads) < 2 {
		t.Fatalf("ReadCodexContext calls = %v, want at least one from each invocation", gotContextReads)
	}
	for _, read := range gotContextReads {
		if read.cwd != cwd || read.sessionID != sessionID {
			t.Errorf("ReadCodexContext call = %+v, want cwd %q session %q", read, cwd, sessionID)
		}
	}

	// Acceptance criterion: the final commit lands with all three
	// Ralph-loop trailers, attributed to the one original session.
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
	// Exactly one commit landed for this ticket: cherry-picking twice would
	// duplicate the trailer/SHA.
	log := exec.Command("git", "log", "--oneline", "--grep", ticketTrailerKey+": "+ticketTrailerValue(epicName, "01"))
	log.Dir = featurePath
	logOut, err := log.Output()
	if err != nil {
		t.Fatalf("git log --grep: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(logOut)), "\n") + 1; strings.TrimSpace(string(logOut)) == "" {
		t.Errorf("landed commits for ticket 01 = 0, want exactly one")
	} else if lines != 1 {
		t.Errorf("landed commits for ticket 01 = %d, want exactly one", lines)
	}

	// Acceptance criterion: ticket status and session metrics are
	// attributed to the one consistent session throughout.
	rawTicket, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile completed ticket: %v", err)
	}
	frontmatter, err := schema.ParseTicketFromRaw(string(rawTicket), ticketPath)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if err := schema.Validate(frontmatter); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if fmt.Sprint(frontmatter.Status) != "done" {
		t.Errorf("ticket status = %v, want done", frontmatter.Status)
	}
	if strings.Contains(string(rawTicket), "status: claimed") {
		t.Errorf("completed ticket = %s, must not still be claimed", rawTicket)
	}
	if frontmatter.ActualContextWindow <= 0 {
		t.Errorf("ActualContextWindow = %d, want a positive value stamped from the original session", frontmatter.ActualContextWindow)
	}

	// Acceptance criterion: worktree, tab, and branch cleanup happened
	// exactly once.
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

	events, ok, err := ReadEvents(scratchDir, epicName)
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	for _, event := range events {
		if event.Type == eventNeedsAnswer || event.Type == eventNeedsRepair || event.Type == eventSmartZoneRecoveryFailed || event.Type == eventPausedSmartZone {
			t.Errorf("unexpected recovery/failure residue event (this scenario is restart/reattach only): %+v", event)
		}
	}
	var gotFinish, gotCherryPick int
	for _, event := range events {
		switch event.Type {
		case eventIterationFinished:
			gotFinish++
		case eventCherryPicked:
			gotCherryPick++
			if event.SHA != sha {
				t.Errorf("cherry-picked SHA = %q, want landed SHA %q", event.SHA, sha)
			}
		}
	}
	if gotFinish != 1 {
		t.Errorf("iteration-finished events = %d, want exactly one across both invocations", gotFinish)
	}
	if gotCherryPick != 1 {
		t.Errorf("cherry-picked events = %d, want exactly one landing", gotCherryPick)
	}
}
