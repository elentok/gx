package herdrfake

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
)

// startKeyAgent starts a claude agent named "claude" on a fresh agent host and
// returns its pane id.
func startKeyAgent(t *testing.T, kind string) (*State, string) {
	t.Helper()
	state := NewState(t)
	if kind == "codex" {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CODEX_HOME", t.TempDir())
		RegisterCodexRollout(t, state, CodexRolloutOptions{
			Cwd: t.TempDir(), PaneID: "p1", AgentID: "a1", AgentName: "agent",
		})
		StartState(t, state)
		agent, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "agent", Kind: "codex", Pane: "p1"})
		if err != nil {
			t.Fatalf("AgentStart: %v", err)
		}
		return state, agent.PaneID
	}
	StartAgentHost(t, state)
	state.Workspaces["w1"] = &Workspace{ID: "w1"}
	state.Tabs["t1"] = &Tab{ID: "t1", WorkspaceID: "w1"}
	state.Panes["p1"] = &Pane{ID: "p1", TabID: "t1"}
	agent, err := herdr.AgentStart(herdr.AgentStartOptions{Name: "agent", Kind: "claude", Pane: "p1"})
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	return state, agent.PaneID
}

func sendKeys(t *testing.T, pane string, keys ...string) {
	t.Helper()
	if err := herdr.AgentSendKeys(pane, keys...); err != nil {
		t.Fatalf("AgentSendKeys(%v): %v", keys, err)
	}
}

func status(t *testing.T, s *State, name string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findAgent(name)
	if a == nil {
		return "<gone>"
	}
	return a.Status
}

func prompt(t *testing.T, pane, text string) {
	t.Helper()
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: pane, Text: text}); err != nil {
		t.Fatalf("AgentPrompt: %v", err)
	}
}

func TestKeysAreRecordedInOrder(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, pane := startKeyAgent(t, kind)
			prompt(t, pane, "work")
			sendKeys(t, pane, "ctrl+u", "esc")
			sendKeys(t, pane, "ctrl+c")
			got, err := state.Keys("agent")
			if err != nil || !reflect.DeepEqual(got, []string{"ctrl+u", "esc", "ctrl+c"}) {
				t.Fatalf("Keys = %v, %v; want ctrl+u esc ctrl+c", got, err)
			}
		})
	}
}

func TestClaudeCtrlCOnWorkingGoesIdle(t *testing.T) {
	state, pane := startKeyAgent(t, "claude")
	prompt(t, pane, "work")
	sendKeys(t, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "idle" {
		t.Fatalf("status = %q, want idle", got)
	}
}

func TestClaudeCtrlCOnIdleKeepsAgent(t *testing.T) {
	state, pane := startKeyAgent(t, "claude")
	sendKeys(t, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "idle" {
		t.Fatalf("status = %q, want idle", got)
	}
}

func TestEscOnBlockedClosesDialog(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, pane := startKeyAgent(t, kind)
			if err := state.SetAgentStatus("agent", "blocked", "input_needed"); err != nil {
				t.Fatal(err)
			}
			sendKeys(t, pane, "esc")
			if got := status(t, state, "agent"); got != "idle" {
				t.Fatalf("status after esc = %q, want idle", got)
			}
		})
	}
}

func TestOtherKeyOnBlockedClaudeContinuesTurn(t *testing.T) {
	state, pane := startKeyAgent(t, "claude")
	if err := state.SetAgentStatus("agent", "blocked", "input_needed"); err != nil {
		t.Fatal(err)
	}
	sendKeys(t, pane, "enter")
	if got := status(t, state, "agent"); got != "working" {
		t.Fatalf("status after enter = %q, want working", got)
	}
}

func TestCtrlUClearsInputLine(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, pane := startKeyAgent(t, kind)
			if err := state.SetInput("agent", "half-typed"); err != nil {
				t.Fatal(err)
			}
			sendKeys(t, pane, "ctrl+u")
			if got, err := state.Input("agent"); err != nil || got != "" {
				t.Fatalf("Input = %q, %v; want empty", got, err)
			}
		})
	}
}

func TestCodexCtrlCOnWorkingGoesDone(t *testing.T) {
	state, pane := startKeyAgent(t, "codex")
	prompt(t, pane, "work")
	sendKeys(t, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
}

func TestCodexCtrlCOnIdleQuits(t *testing.T) {
	state, pane := startKeyAgent(t, "codex")
	sendKeys(t, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "<gone>" {
		t.Fatalf("status = %q, want agent gone", got)
	}
	requireNotFound(t, herdr.AgentSendKeys(pane, "ctrl+u"))
}

func TestVanishAfterKeyInjectsAgentNotFound(t *testing.T) {
	state, pane := startKeyAgent(t, "claude")
	if err := state.VanishAfterKey("agent", "ctrl+c"); err != nil {
		t.Fatal(err)
	}
	sendKeys(t, pane, "ctrl+c")
	_, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: pane, Until: []string{"idle"}})
	requireNotFound(t, err)
}

func TestDoneAndUnknownStatesCanBeSet(t *testing.T) {
	state, pane := startKeyAgent(t, "claude")
	for _, want := range []string{"done", "unknown"} {
		if err := state.SetAgentStatus("agent", want, ""); err != nil {
			t.Fatal(err)
		}
		got, err := herdr.AgentGet(pane)
		if err != nil || got.AgentStatus != want {
			t.Fatalf("AgentGet = %+v, %v; want status %q", got, err, want)
		}
	}
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "agent_not_found") {
		t.Fatalf("error = %v, want agent_not_found", err)
	}
}
