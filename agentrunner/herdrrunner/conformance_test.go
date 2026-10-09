package herdrrunner_test

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
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
		return runnertest.Harness{
			Runner: herdrrunner.New(),
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				setStatus(t, s, "idle", "")
			},
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				setStatus(t, s, "blocked", reason)
			},
			RateLimit: func(t *testing.T, s agentrunner.Session, resetAt time.Time) {
				t.Skip("herdr RateLimit lands in a sibling ticket")
			},
			Stall: func(t *testing.T, s agentrunner.Session) {
				t.Skip("herdrfake stall support lands in a follow-up ticket")
			},
			BackgroundTask: func(t *testing.T, s agentrunner.Session, running bool) {
				t.Skip("herdr background-task gating lands in a follow-up ticket")
			},
			Restart: func(t *testing.T) agentrunner.Runner { return herdrrunner.New() },
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

func TestFind_UnknownLabel(t *testing.T) {
	herdrfake.StartAgentHost(t, herdrfake.NewState(t))
	_, ok, err := herdrrunner.New().Find("nobody")
	if err != nil || ok {
		t.Fatalf("Find = ok %v, err %v; want not found, nil", ok, err)
	}
}
