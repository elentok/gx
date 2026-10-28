package herdrrunner_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil/claudeenv"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	home, err := os.MkdirTemp("", "gx-herdrrunner")
	if err != nil {
		panic(err)
	}
	cleanEnv = claudeenv.In(home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// cleanEnv points HOME at a poisoned home and CLAUDE_CONFIG_DIR at a clean
// config, so a runner reading claude's config from the real home fails loudly.
var cleanEnv []string

// testEnv is the process env with cleanEnv on top. It is read at call time:
// herdrfake publishes its socket through the process env.
func testEnv() []string { return append(os.Environ(), cleanEnv...) }

// newTestRunner is a runner under testEnv, without background-task gating.
func newTestRunner() *herdrrunner.Runner {
	return herdrrunner.NewIn(herdr.CLI{Env: testEnv()}, "")
}

func TestConformance(t *testing.T) {
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		state := herdrfake.NewState(t)
		herdrfake.StartAgentHost(t, state)
		setStatus := func(t *testing.T, s agentrunner.Session, status, rule string) {
			if err := state.SetAgentStatus(s.Label, status, rule); err != nil {
				t.Fatal(err)
			}
		}
		var tasksMu sync.Mutex
		tasks := map[string]bool{}
		newRunner := func() agentrunner.Runner {
			r := newTestRunner()
			r.N = 200 * time.Millisecond
			r.BackgroundTasks = func(s agentrunner.Session) bool {
				tasksMu.Lock()
				defer tasksMu.Unlock()
				return tasks[s.Label]
			}
			return r
		}
		return runnertest.Harness{
			Runner: newRunner(),
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				setStatus(t, s, "idle", "")
			},
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				setStatus(t, s, "blocked", reason)
			},
			BlockOnDialog: func(t *testing.T, s agentrunner.Session, permission bool) {
				rule := "input_needed"
				if permission {
					rule = "permission_prompt"
				}
				setStatus(t, s, "blocked", rule)
			},
			LastPrompt: func(t *testing.T, s agentrunner.Session) string {
				got, err := state.Received(s.Label)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) == 0 {
					return ""
				}
				return got[len(got)-1]
			},
			RateLimit: func(t *testing.T, s agentrunner.Session, resetAt time.Time) {
				if err := state.SetPaneText(s.Label, "You've hit your session limit · resets "+resetAt.Format("3:04pm")); err != nil {
					t.Fatal(err)
				}
			},
			Stall: func(t *testing.T, s agentrunner.Session) {
				if err := state.StallAgent(s.Label); err != nil {
					t.Fatal(err)
				}
			},
			BackgroundTask: func(t *testing.T, s agentrunner.Session, running bool) {
				tasksMu.Lock()
				defer tasksMu.Unlock()
				tasks[s.Label] = running
			},
			Received: func(t *testing.T, s agentrunner.Session) string {
				got, err := state.Received(s.Label)
				if err != nil {
					t.Fatal(err)
				}
				keys, err := state.Keys(s.Label)
				if err != nil {
					t.Fatal(err)
				}
				// Keys follow the prompts, as the pty harness reports raw input.
				raw := map[string]string{"ctrl+c": "\x03", "esc": "\x1b", "ctrl+u": "\x15"}
				out := strings.Join(got, "\n")
				for _, k := range keys {
					out += raw[k]
				}
				return out
			},
			FastTurn: func(t *testing.T, s agentrunner.Session) {
				if err := state.FastTurn(s.Label); err != nil {
					t.Fatal(err)
				}
			},
			OpenDialogOnNextPrompt: func(t *testing.T, s agentrunner.Session) {
				if err := state.OpenDialogOnNextSlash(s.Label); err != nil {
					t.Fatal(err)
				}
			},
			Restart: func(t *testing.T) agentrunner.Runner { return newRunner() },
			Skip: map[string]string{
				"OtherDialogAllowRefused": "herdr Answer does not tell permission prompts from other dialogs",
			},
		}
	})
}

// ralph-loop's server launch labels the tab after the ticket but names the
// agent after the iteration, so List must not take tab labels as agent names.
func TestList_TabLabelDiffersFromAgentName(t *testing.T) {
	herdrfake.StartAgentHost(t, herdrfake.NewState(t))
	wsID, err := herdr.EnsureWorkspace("epic", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tab, err := herdr.TabCreate(herdr.TabCreateOptions{WorkspaceID: wsID, Label: "epic/03"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "epic-iter-03", Kind: "claude", Pane: tab.RootPaneID})
	if err != nil {
		t.Fatal(err)
	}

	got, err := newTestRunner().List("epic")
	if err != nil {
		t.Fatal(err)
	}
	want := []agentrunner.Session{{Label: "epic-iter-03", ID: agent.PaneID}}
	if !slices.Equal(got, want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
}

func startAgent(t *testing.T, kind agentrunner.Kind, quota herdrrunner.CodexQuotaReader) (*herdrrunner.Runner, *herdrfake.State, agentrunner.Session) {
	t.Helper()
	state := herdrfake.NewState(t)
	herdrfake.StartAgentHost(t, state)
	r := newTestRunner()
	r.BackgroundTasks = nil
	r.CodexQuota = quota
	s, err := r.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: kind})
	if err != nil {
		t.Fatal(err)
	}
	return r, state, s
}

func startCodex(t *testing.T, quota herdrrunner.CodexQuotaReader) (*herdrrunner.Runner, *herdrfake.State, agentrunner.Session) {
	t.Helper()
	return startAgent(t, "codex", quota)
}

// A gx restarted mid-turn starts the same label in the same cwd again; it
// gets the live agent back as is, not a fresh one or an idle wait.
func TestStart_AdoptsLiveAgentOnSameCwd(t *testing.T) {
	state := herdrfake.NewState(t)
	herdrfake.StartAgentHost(t, state)
	opts := agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"}
	s, err := newTestRunner().Start(opts)
	if err != nil {
		t.Fatal(err)
	}
	r := newTestRunner()
	if err := r.Prompt(s, "do it"); err != nil {
		t.Fatal(err)
	}
	before, err := r.Status(s)
	if err != nil {
		t.Fatal(err)
	}

	got, err := newTestRunner().Start(opts)
	if err != nil || got.ID != s.ID {
		t.Fatalf("second Start = %+v, %v; want %+v adopted", got, err, s)
	}
	if st, err := r.Status(got); err != nil || st != before {
		t.Fatalf("status after adopt = %+v, %v; want %+v", st, err, before)
	}
}

func TestPrompt_CompactWaitsOutCodexConfirmation(t *testing.T) {
	herdrrunner.ShortenCompactTimings(t)
	r, state, s := startCodex(t, nil)
	if err := state.ConfirmCompact(s.Label); err != nil {
		t.Fatal(err)
	}
	// Answer the confirmation only once the fake has raised it: a fixed sleep
	// can fire before a slow "agent prompt" dispatch, which then re-blocks
	// the agent with nobody left to move it on.
	done := make(chan struct{})
	go func() {
		for !promptDispatched(state, "/compact") {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
		_ = state.SetAgentStatus(s.Label, "working", "")
	}()
	err := r.Prompt(s, "/compact")
	close(done)
	if err != nil {
		t.Fatalf("Prompt(/compact) = %v, want nil", err)
	}
	if st, err := r.Status(s); err != nil || st.State != agentrunner.StateWorking {
		t.Fatalf("status after /compact = %+v, %v; want working", st, err)
	}
}

// promptDispatched reports whether the fake has handled an "agent prompt"
// carrying text.
func promptDispatched(state *herdrfake.State, text string) bool {
	return slices.ContainsFunc(state.Trace(), func(e herdrfake.TraceEntry) bool {
		return len(e.Argv) > 3 && e.Argv[0] == "agent" && e.Argv[1] == "prompt" && e.Argv[3] == text
	})
}

// Codex's /status starts no turn and moves no seq: it is delivered once the
// agent is still idle after N, and never retyped. /new moves the seq, so it
// returns without waiting out N.
func TestPrompt_CodexSlashCommands(t *testing.T) {
	r, state, s := startCodex(t, nil)
	r.N = 300 * time.Millisecond

	start := time.Now()
	if err := r.Prompt(s, "/new"); err != nil {
		t.Fatalf("Prompt(/new) = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed >= r.N {
		t.Fatalf("Prompt(/new) took %v, want it to return on the seq move before N=%v", elapsed, r.N)
	}

	start = time.Now()
	if err := r.Prompt(s, "/status"); err != nil {
		t.Fatalf("Prompt(/status) = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < r.N {
		t.Fatalf("Prompt(/status) took %v, want it to wait out N=%v", elapsed, r.N)
	}
	got, err := state.Received(s.Label)
	if err != nil || !slices.Equal(got, []string{"/new", "/status"}) {
		t.Fatalf("received = %q, %v; want each command once", got, err)
	}
}

func TestPrompt_CompactConfirmationNeverAnsweredIsNotDelivered(t *testing.T) {
	herdrrunner.ShortenCompactTimings(t)
	r, state, s := startCodex(t, nil)
	if err := state.ConfirmCompact(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := r.Prompt(s, "/compact"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt(/compact) = %v, want ErrNotDelivered", err)
	}
}

func TestPrompt_CompactWaitsForPaneToShowItSubmitted(t *testing.T) {
	herdrrunner.ShortenCompactTimings(t)
	r, state, s := startAgent(t, "claude", nil)
	if err := state.SetPaneText(s.Label, "earlier output\n/compact\n"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = state.SetPaneText(s.Label, "/compact\n✻ Compacting conversation…")
	}()
	if err := r.Prompt(s, "/compact"); err != nil {
		t.Fatalf("Prompt(/compact) = %v, want nil", err)
	}
}

func TestPrompt_CompactNeverSubmittedIsNotDelivered(t *testing.T) {
	herdrrunner.ShortenCompactTimings(t)
	r, state, s := startAgent(t, "claude", nil)
	if err := state.SetPaneText(s.Label, "/compact"); err != nil {
		t.Fatal(err)
	}
	if err := r.Prompt(s, "/compact"); !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Prompt(/compact) = %v, want ErrNotDelivered", err)
	}
}

func TestRateLimit_CodexQuotaRecordWinsOverPane(t *testing.T) {
	resetAt := time.Date(2026, 8, 5, 15, 6, 0, 0, time.UTC)
	r, _, s := startCodex(t, func(string, string) (codexsession.RateLimit, bool, error) {
		return codexsession.RateLimit{Quota: "usage", ResetAt: resetAt}, true, nil
	})
	s.SessionID = "session-1"
	got, limited, err := r.RateLimit(s)
	if err != nil || !limited || !got.Equal(resetAt) {
		t.Fatalf("RateLimit = %v, %v, %v; want %v, limited", got, limited, err, resetAt)
	}
}

func TestRateLimit_CodexPaneQuota(t *testing.T) {
	r, state, s := startCodex(t, nil)
	if err := state.SetPaneText(s.Label, "■ You've hit your usage limit. Try again in 2 hours."); err != nil {
		t.Fatal(err)
	}
	got, limited, err := r.RateLimit(s)
	if err != nil || !limited {
		t.Fatalf("RateLimit = %v, %v; want limited", limited, err)
	}
	if d := time.Until(got) - 2*time.Hour; d.Abs() > time.Minute {
		t.Fatalf("RateLimit resetAt = %v, want ~2h from now", got)
	}
}

func TestRateLimit_CodexContextExhausted(t *testing.T) {
	r, state, s := startCodex(t, nil)
	if err := state.SetPaneText(s.Label, "■ Codex ran out of room in the model's context window."); err != nil {
		t.Fatal(err)
	}
	_, limited, err := r.RateLimit(s)
	if !errors.Is(err, agentrunner.ErrContextExhausted) || limited {
		t.Fatalf("RateLimit = %v, %v; want ErrContextExhausted, not limited", limited, err)
	}
	// ralph-loop puts the evidence in the pause reason.
	if !strings.Contains(err.Error(), "ran out of room") {
		t.Fatalf("RateLimit err = %q, want the pane evidence", err)
	}
}

func TestRateLimit_CodexIncidentalPaneTextIsNeither(t *testing.T) {
	for _, text := range []string{
		"Error: request failed with status 500",
		`I am adding detection for "Your input exceeds the context window of this model."`,
		"The response included context_length_exceeded, which we should classify.",
		"blocked: approve this command",
	} {
		r, state, s := startCodex(t, nil)
		if err := state.SetPaneText(s.Label, text); err != nil {
			t.Fatal(err)
		}
		if _, limited, err := r.RateLimit(s); err != nil || limited {
			t.Errorf("RateLimit(%q) = limited %v, err %v; want neither", text, limited, err)
		}
	}
}

func TestRateLimit_CodexQuotaReadErrorIsReturned(t *testing.T) {
	boom := errors.New("rollout unreadable")
	r, _, s := startCodex(t, func(string, string) (codexsession.RateLimit, bool, error) {
		return codexsession.RateLimit{}, false, boom
	})
	s.SessionID = "session-1"
	if _, _, err := r.RateLimit(s); !errors.Is(err, boom) {
		t.Fatalf("RateLimit err = %v, want %v", err, boom)
	}
}

func TestFind_UnknownLabel(t *testing.T) {
	herdrfake.StartAgentHost(t, herdrfake.NewState(t))
	_, ok, err := newTestRunner().Find("nobody")
	if err != nil || ok {
		t.Fatalf("Find = ok %v, err %v; want not found, nil", ok, err)
	}
}

// The adapter owns the tab: callers never see it, so Stop closes it.
func TestStop_ClosesTheAgentsTab(t *testing.T) {
	r, state, s := startAgent(t, "claude", nil)
	var closed []string
	state.Register("tab", "close", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		closed = append(closed, argv[len(argv)-1])
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	agent, err := herdr.AgentGet(s.ID)
	if err != nil || agent.TabID == "" {
		t.Fatalf("AgentGet = %+v, %v; want a tab", agent, err)
	}
	if err := r.Stop(s); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(closed, []string{agent.TabID}) {
		t.Errorf("closed tabs = %v, want [%s]", closed, agent.TabID)
	}
}

func TestStop_ReportsAFailedTabLookup(t *testing.T) {
	r, state, s := startAgent(t, "claude", nil)
	state.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return nil, herdrfake.Identities{}, errors.New("herdr unreachable")
	})
	if err := r.Stop(s); err == nil {
		t.Fatal("Stop = nil, want the lookup error")
	}
}

// An agent herdr shows idle but a background task keeps working answers every
// wait at once; Wait must not re-ask at its 20ms pace, since each round spawns
// herdr twice and parses the transcript (a busy loop for the task's lifetime).
func TestWait_BackgroundTaskPollsSlowly(t *testing.T) {
	r, _, s := startAgent(t, "claude", nil)
	probes := 0
	r.BackgroundTasks = func(agentrunner.Session) bool {
		probes++
		return true
	}
	now := time.Now()
	r.Now = func() time.Time { return now }
	r.Sleep = func(d time.Duration) { now = now.Add(d) }

	_, err := r.Wait(s, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, 10*time.Second)
	if !errors.Is(err, agentrunner.ErrTimeout) {
		t.Fatalf("Wait = %v, want ErrTimeout", err)
	}
	if probes > 10 {
		t.Fatalf("probed background tasks %d times in 10s, want at most 10", probes)
	}
}

func denyWithReason(t *testing.T, kind agentrunner.Kind) {
	t.Helper()
	r, state, s := startAgent(t, kind, nil)
	if err := state.SetAgentStatus(s.Label, "blocked", "permission_prompt"); err != nil {
		t.Fatal(err)
	}
	const reason = "use rg"
	if err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: reason}); err != nil {
		t.Fatal(err)
	}
	keys, _ := state.Keys(s.Label)
	if !slices.Equal(keys, []string{"esc"}) {
		t.Fatalf("keys = %v, want only esc: the reason is never dialog keys", keys)
	}
	got, _ := state.Received(s.Label)
	if !slices.Equal(got, []string{reason}) {
		t.Fatalf("prompts = %v, want the reason once", got)
	}
	if st, err := r.Status(s); err != nil || st.State != agentrunner.StateWorking {
		t.Fatalf("Status after the deny = %+v, %v; want the hold gone and the agent working", st, err)
	}
}

func TestAnswer_DenyWithReasonEscThenPrompt(t *testing.T) {
	denyWithReason(t, "claude")
}

func TestAnswer_DenyWithReasonEscThenPromptCodex(t *testing.T) {
	denyWithReason(t, "codex")
}

// A dialog that stays open through Esc leaves the reason unsent: the error says
// so, the hold is gone and the session still takes verbs.
func TestAnswer_DenyWithReasonFailureAfterEscIsNotDelivered(t *testing.T) {
	r, state, s := startAgent(t, "claude", nil)
	r.N = 50 * time.Millisecond
	if err := state.StallAgent(s.Label); err != nil {
		t.Fatal(err)
	}
	if err := state.SetAgentStatus(s.Label, "blocked", "permission_prompt"); err != nil {
		t.Fatal(err)
	}
	err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "use rg"})
	if !errors.Is(err, agentrunner.ErrNotDelivered) || !strings.Contains(err.Error(), "denied, but the reason was not delivered") {
		t.Fatalf("Answer err = %v, want ErrNotDelivered saying the reason was not delivered", err)
	}
	if errors.Is(err, agentrunner.ErrNotReady) {
		t.Fatalf("Answer err = %v, must not also match ErrNotReady", err)
	}
	if got, _ := state.Received(s.Label); len(got) != 0 {
		t.Fatalf("prompts = %v, want none", got)
	}
	// The hold ended with the error: a plain allow reaches the dialog again.
	if err := r.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err != nil {
		t.Fatalf("Answer after the failed deny: %v", err)
	}
	if keys, _ := state.Keys(s.Label); !slices.Equal(keys, []string{"esc", "enter"}) {
		t.Fatalf("keys = %v, want esc then enter", keys)
	}
}
