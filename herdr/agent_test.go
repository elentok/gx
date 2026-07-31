package herdr

import (
	"errors"
	"strings"
	"testing"
)

func TestAgentPrompt_ParsesAgentSession(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"agent_prompted",
			"agent":{
				"pane_id":"wE:p1","workspace_id":"wE","tab_id":"wE:t9","agent_status":"idle",
				"agent_session":{"source":"herdr:claude","agent":"claude","kind":"id","value":"f6be0343-85c1-4a50-8911-927380626a6b"}
			}
		}}`), nil
	})
	agent, err := AgentPrompt(AgentPromptOptions{
		Target:    "wE:p1",
		Text:      "/implement .scratch/foo/issues/01-x.md",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 5000,
	})
	if err != nil {
		t.Fatalf("AgentPrompt() error = %v", err)
	}
	want := Agent{
		PaneID:       "wE:p1",
		WorkspaceID:  "wE",
		TabID:        "wE:t9",
		AgentStatus:  "idle",
		AgentSession: "f6be0343-85c1-4a50-8911-927380626a6b",
	}
	if agent != want {
		t.Fatalf("AgentPrompt() = %+v, want %+v", agent, want)
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{
		"agent prompt wE:p1 /implement .scratch/foo/issues/01-x.md",
		"--wait", "--until idle", "--until done", "--timeout 5000",
	} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
}

func TestAgentPrompt_NoWait_OmitsFlags(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"agent":{"pane_id":"wE:p1","workspace_id":"wE","tab_id":"wE:t9","agent_status":"working"}}}`), nil
	})
	agent, err := AgentPrompt(AgentPromptOptions{Target: "wE:p1", Text: "hi"})
	if err != nil {
		t.Fatalf("AgentPrompt() error = %v", err)
	}
	if agent.AgentSession != "" {
		t.Fatalf("AgentPrompt().AgentSession = %q, want empty when agent_session is null", agent.AgentSession)
	}
	if strings.Contains(strings.Join(gotArgs, " "), "--wait") {
		t.Fatalf("args %v should not contain --wait", gotArgs)
	}
}

func TestAgentPrompt_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := AgentPrompt(AgentPromptOptions{Target: "wE:p1", Text: "hi"}); err == nil {
		t.Fatal("AgentPrompt() error = nil, want error")
	}
}

func TestAgentStart_ParsesAgentSession(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"agent_started",
			"argv":["claude"],
			"agent":{
				"pane_id":"wE:p1","workspace_id":"wE","tab_id":"wE:t9","agent_status":"idle",
				"agent_session":{"source":"herdr:claude","agent":"claude","kind":"id","value":"f6be0343-85c1-4a50-8911-927380626a6b"}
			}
		}}`), nil
	})
	agent, err := AgentStart(AgentStartOptions{Name: "iter-01", Kind: "claude", Pane: "wE:p1", TimeoutMs: 30000})
	if err != nil {
		t.Fatalf("AgentStart() error = %v", err)
	}
	want := Agent{
		PaneID:       "wE:p1",
		WorkspaceID:  "wE",
		TabID:        "wE:t9",
		AgentStatus:  "idle",
		AgentSession: "f6be0343-85c1-4a50-8911-927380626a6b",
	}
	if agent != want {
		t.Fatalf("AgentStart() = %+v, want %+v", agent, want)
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{"agent start iter-01 --kind claude --pane wE:p1", "--timeout 30000"} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
}

func TestAgentStart_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := AgentStart(AgentStartOptions{Name: "iter-01", Kind: "claude", Pane: "wE:p1"}); err == nil {
		t.Fatal("AgentStart() error = nil, want error")
	}
}

func TestAgentWait_BuildsArgsAndParses(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"agent_info",
			"agent":{
				"pane_id":"wE:p1","workspace_id":"wE","tab_id":"wE:t9","agent_status":"idle",
				"agent_session":{"source":"herdr:claude","agent":"claude","kind":"id","value":"f6be0343-85c1-4a50-8911-927380626a6b"}
			}
		}}`), nil
	})
	agent, err := AgentWait(AgentWaitOptions{Target: "wE:p1", Until: []string{"idle", "done"}, TimeoutMs: 5000})
	if err != nil {
		t.Fatalf("AgentWait() error = %v", err)
	}
	if agent.AgentStatus != "idle" {
		t.Fatalf("AgentWait().AgentStatus = %q, want %q", agent.AgentStatus, "idle")
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{"agent wait wE:p1", "--until idle", "--until done", "--timeout 5000"} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
}

func TestAgentWait_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := AgentWait(AgentWaitOptions{Target: "wE:p1"}); err == nil {
		t.Fatal("AgentWait() error = nil, want error")
	}
}

func TestAgentSendKeys_BuildsArgs(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"type":"ok"}}`), nil
	})
	if err := AgentSendKeys("wE:p1", "ctrl-c"); err != nil {
		t.Fatalf("AgentSendKeys() error = %v", err)
	}
	want := []string{"agent", "send-keys", "wE:p1", "ctrl-c"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("AgentSendKeys() args = %v, want %v", gotArgs, want)
	}
}

func TestAgentSendKeys_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if err := AgentSendKeys("wE:p1", "ctrl-c"); err == nil {
		t.Fatal("AgentSendKeys() error = nil, want error")
	}
}

func TestAgentRead_ReturnsRawText(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("some terminal output\n"), nil
	})
	out, err := AgentRead("wE:p1", AgentReadOptions{Source: "recent-unwrapped", Lines: 50, Format: "text"})
	if err != nil {
		t.Fatalf("AgentRead() error = %v", err)
	}
	if out != "some terminal output\n" {
		t.Fatalf("AgentRead() = %q", out)
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{"agent read wE:p1", "--source recent-unwrapped", "--lines 50", "--format text"} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
}

func TestAgentRead_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := AgentRead("wE:p1", AgentReadOptions{}); err == nil {
		t.Fatal("AgentRead() error = nil, want error")
	}
}
