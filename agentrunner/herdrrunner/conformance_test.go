package herdrrunner_test

import (
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
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
			r := herdrrunner.New()
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
			Restart: func(t *testing.T) agentrunner.Runner { return newRunner() },
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

	got, err := herdrrunner.New().List("epic")
	if err != nil {
		t.Fatal(err)
	}
	want := []agentrunner.Session{{Label: "epic-iter-03", ID: agent.PaneID}}
	if !slices.Equal(got, want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
}

func startCodex(t *testing.T, quota herdrrunner.CodexQuotaReader) (*herdrrunner.Runner, *herdrfake.State, agentrunner.Session) {
	t.Helper()
	state := herdrfake.NewState(t)
	herdrfake.StartAgentHost(t, state)
	r := herdrrunner.New()
	r.BackgroundTasks = nil
	r.CodexQuota = quota
	s, err := r.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	return r, state, s
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
}

func TestFind_UnknownLabel(t *testing.T) {
	herdrfake.StartAgentHost(t, herdrfake.NewState(t))
	_, ok, err := herdrrunner.New().Find("nobody")
	if err != nil || ok {
		t.Fatalf("Find = ok %v, err %v; want not found, nil", ok, err)
	}
}
