package herdrfake

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
)

// startStateCLI serves state as a fake herdr and returns the CLI that reaches it.
func startStateCLI(t *testing.T, state *State) herdr.CLI {
	t.Helper()
	_, env := StartStateEnv(t, state)
	return herdr.CLI{Env: append(os.Environ(), env...)}
}

// startKeyAgent starts a claude agent named "claude" on a fresh agent host and
// returns the CLI that reaches it and its pane id.
func startKeyAgent(t *testing.T, kind string) (*State, herdr.CLI, string) {
	t.Helper()
	state := NewState(t)
	if kind == "codex" {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CODEX_HOME", t.TempDir())
		RegisterCodexRollout(t, state, CodexRolloutOptions{
			Cwd: t.TempDir(), PaneID: "p1", AgentID: "a1", AgentName: "agent",
		})
		_, env := StartStateEnv(t, state)
		cli := herdr.CLI{Env: append(os.Environ(), env...)}
		agent, err := cli.AgentStart(herdr.AgentStartOptions{Name: "agent", Kind: "codex", Pane: "p1"})
		if err != nil {
			t.Fatalf("AgentStart: %v", err)
		}
		return state, cli, agent.PaneID
	}
	_, env := StartAgentHostEnv(t, state)
	cli := herdr.CLI{Env: append(os.Environ(), env...)}
	state.Seed(func(s *State) {
		s.Workspaces["w1"] = &Workspace{ID: "w1"}
		s.Tabs["t1"] = &Tab{ID: "t1", WorkspaceID: "w1"}
		s.Panes["p1"] = &Pane{ID: "p1", TabID: "t1"}
	})
	agent, err := cli.AgentStart(herdr.AgentStartOptions{Name: "agent", Kind: "claude", Pane: "p1"})
	if err != nil {
		t.Fatalf("AgentStart: %v", err)
	}
	return state, cli, agent.PaneID
}

func sendKeys(t *testing.T, cli herdr.CLI, pane string, keys ...string) {
	t.Helper()
	if err := cli.AgentSendKeys(pane, keys...); err != nil {
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

func prompt(t *testing.T, cli herdr.CLI, pane, text string) {
	t.Helper()
	if _, err := cli.AgentPrompt(herdr.AgentPromptOptions{Target: pane, Text: text}); err != nil {
		t.Fatalf("AgentPrompt: %v", err)
	}
}

func TestKeysAreRecordedInOrder(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, cli, pane := startKeyAgent(t, kind)
			prompt(t, cli, pane, "work")
			sendKeys(t, cli, pane, "ctrl+u", "esc")
			sendKeys(t, cli, pane, "ctrl+c")
			got, err := state.Keys("agent")
			if err != nil || !reflect.DeepEqual(got, []string{"ctrl+u", "esc", "ctrl+c"}) {
				t.Fatalf("Keys = %v, %v; want ctrl+u esc ctrl+c", got, err)
			}
		})
	}
}

func TestClaudeCtrlCOnWorkingGoesIdle(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "claude")
	prompt(t, cli, pane, "work")
	sendKeys(t, cli, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "idle" {
		t.Fatalf("status = %q, want idle", got)
	}
}

func TestClaudeCtrlCOnIdleKeepsAgent(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "claude")
	sendKeys(t, cli, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "idle" {
		t.Fatalf("status = %q, want idle", got)
	}
}

func TestEscOnBlockedClosesDialog(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, cli, pane := startKeyAgent(t, kind)
			if err := state.SetAgentStatus("agent", "blocked", "input_needed"); err != nil {
				t.Fatal(err)
			}
			sendKeys(t, cli, pane, "esc")
			if got := status(t, state, "agent"); got != "idle" {
				t.Fatalf("status after esc = %q, want idle", got)
			}
		})
	}
}

func TestOtherKeyOnBlockedClaudeContinuesTurn(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "claude")
	if err := state.SetAgentStatus("agent", "blocked", "input_needed"); err != nil {
		t.Fatal(err)
	}
	sendKeys(t, cli, pane, "enter")
	if got := status(t, state, "agent"); got != "working" {
		t.Fatalf("status after enter = %q, want working", got)
	}
}

func TestCtrlUClearsInputLine(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			state, cli, pane := startKeyAgent(t, kind)
			if err := state.SetInput("agent", "half-typed"); err != nil {
				t.Fatal(err)
			}
			sendKeys(t, cli, pane, "ctrl+u")
			if got, err := state.Input("agent"); err != nil || got != "" {
				t.Fatalf("Input = %q, %v; want empty", got, err)
			}
		})
	}
}

func TestCodexCtrlCOnWorkingGoesDone(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "codex")
	prompt(t, cli, pane, "work")
	sendKeys(t, cli, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
}

func TestCodexCtrlCOnIdleQuits(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "codex")
	sendKeys(t, cli, pane, "ctrl+c")
	if got := status(t, state, "agent"); got != "<gone>" {
		t.Fatalf("status = %q, want agent gone", got)
	}
	requireNotFound(t, cli.AgentSendKeys(pane, "ctrl+u"))
}

func TestVanishAfterKeyInjectsAgentNotFound(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "claude")
	if err := state.VanishAfterKey("agent", "ctrl+c"); err != nil {
		t.Fatal(err)
	}
	sendKeys(t, cli, pane, "ctrl+c")
	_, err := cli.AgentWait(herdr.AgentWaitOptions{Target: pane, Until: []string{"idle"}})
	requireNotFound(t, err)
}

func TestDoneAndUnknownStatesCanBeSet(t *testing.T) {
	state, cli, pane := startKeyAgent(t, "claude")
	for _, want := range []string{"done", "unknown"} {
		if err := state.SetAgentStatus("agent", want, ""); err != nil {
			t.Fatal(err)
		}
		got, err := cli.AgentGet(pane)
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
