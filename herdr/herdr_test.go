package herdr

import (
	"errors"
	"strings"
	"testing"
)

// withFakeCommand swaps runCommand for fn for the duration of the test.
func withFakeCommand(t *testing.T, fn func(args ...string) ([]byte, error)) {
	t.Helper()
	prev := runCommand
	runCommand = fn
	t.Cleanup(func() { runCommand = prev })
}

func TestRun_Success_ReturnsOutput(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("ok"), nil
	})
	out, err := run("workspace", "list")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("run() = %q, want %q", out, "ok")
	}
}

func TestRun_Failure_WrapsCommandAndOutput(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	_, err := run("workspace", "list")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "$ herdr workspace list") {
		t.Errorf("error missing command line: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error missing output: %v", err)
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("error missing underlying error: %v", err)
	}
}

func TestRunJSON_ParsesResultField(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"id":"cli:x","result":{"count":3}}`), nil
	})
	var result struct {
		Count int `json:"count"`
	}
	if err := runJSON([]string{"x"}, &result); err != nil {
		t.Fatalf("runJSON() error = %v", err)
	}
	if result.Count != 3 {
		t.Fatalf("result.Count = %d, want 3", result.Count)
	}
}

func TestRunJSON_PropagatesCommandError(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"nope"}}`), errors.New("exit 1")
	})
	var result struct{}
	if err := runJSON([]string{"x"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestRunJSON_MalformedOutput_ReturnsError(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("not json"), nil
	})
	var result struct{}
	if err := runJSON([]string{"x"}, &result); err == nil {
		t.Fatal("runJSON() error = nil, want error")
	}
}

func TestRun_AgentNameTakenError_ParsesCodeAndCandidateCwd(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"agent_name_taken","message":"agent name iter-06b is already used; candidates:\nterminal_id=term_6587c212a7704102 pane_id=w1K:pB workspace_id=w1K tab_id=w1K:tB\ncwd=/Users/david/dev/gx/bugs-03-item-06b status=Working"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-06b")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var nameTaken *AgentNameTakenError
	if !errors.As(err, &nameTaken) {
		t.Fatalf("run() error = %v, want an *AgentNameTakenError", err)
	}
	if nameTaken.CandidateCwd != "/Users/david/dev/gx/bugs-03-item-06b" {
		t.Errorf("CandidateCwd = %q, want the candidate's cwd", nameTaken.CandidateCwd)
	}
	if !strings.Contains(nameTaken.Error(), "iter-06b") {
		t.Errorf("Error() = %q, want it to mention the command context", nameTaken.Error())
	}
}

func TestRun_OtherCLIError_NotAgentNameTaken(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"pane_not_found","message":"no such pane"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-01")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var nameTaken *AgentNameTakenError
	if errors.As(err, &nameTaken) {
		t.Errorf("run() error = %v, want a plain error (not agent_name_taken)", err)
	}
}

func TestRun_AgentBlockedError_Classified(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"agent_blocked","message":"agent iter-06b is blocked and not accepting prompts"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "prompt", "iter-06b", "hello")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var blocked *AgentBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("run() error = %v, want an *AgentBlockedError", err)
	}
	if blocked.Message != "agent iter-06b is blocked and not accepting prompts" {
		t.Errorf("Message = %q, want the envelope's message", blocked.Message)
	}
	if !strings.Contains(blocked.Error(), "iter-06b") {
		t.Errorf("Error() = %q, want it to mention the command context", blocked.Error())
	}
}

func TestRun_OtherCLIError_NotAgentBlocked(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"pane_not_found","message":"no such pane"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "prompt", "iter-01", "hello")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var blocked *AgentBlockedError
	if errors.As(err, &blocked) {
		t.Errorf("run() error = %v, want a plain error (not agent_blocked)", err)
	}
}

func TestRun_AgentNameLostError_Classified(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"agent_name_lost","message":"pane for agent iter-06b changed identity before it became ready"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-06b")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var lost *AgentNameLostError
	if !errors.As(err, &lost) {
		t.Fatalf("run() error = %v, want an *AgentNameLostError", err)
	}
	if lost.Message != "pane for agent iter-06b changed identity before it became ready" {
		t.Errorf("Message = %q, want the envelope's message", lost.Message)
	}
	if !strings.Contains(lost.Error(), "iter-06b") {
		t.Errorf("Error() = %q, want it to mention the command context", lost.Error())
	}
}

func TestRun_OtherCLIError_NotAgentNameLost(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"pane_not_found","message":"no such pane"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-01")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var lost *AgentNameLostError
	if errors.As(err, &lost) {
		t.Errorf("run() error = %v, want a plain error (not agent_name_lost)", err)
	}
}

func TestRun_AgentNotReadyError_Classified(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"agent_not_ready","message":"agent iter-06b is blocked during startup and is not ready for prompts"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-06b")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var notReady *AgentNotReadyError
	if !errors.As(err, &notReady) {
		t.Fatalf("run() error = %v, want an *AgentNotReadyError", err)
	}
	if notReady.Message != "agent iter-06b is blocked during startup and is not ready for prompts" {
		t.Errorf("Message = %q, want the envelope's message", notReady.Message)
	}
	if !strings.Contains(notReady.Error(), "iter-06b") {
		t.Errorf("Error() = %q, want it to mention the command context", notReady.Error())
	}
}

func TestRun_OtherCLIError_NotAgentNotReady(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"error":{"code":"pane_not_found","message":"no such pane"}}`), errors.New("exit 1")
	})
	_, err := run("agent", "start", "iter-01")
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	var notReady *AgentNotReadyError
	if errors.As(err, &notReady) {
		t.Errorf("run() error = %v, want a plain error (not agent_not_ready)", err)
	}
}

func TestAppendFlag(t *testing.T) {
	got := appendFlag([]string{"a"}, "--flag", "")
	if len(got) != 1 {
		t.Fatalf("appendFlag with empty value = %v, want unchanged", got)
	}
	got = appendFlag([]string{"a"}, "--flag", "v")
	want := []string{"a", "--flag", "v"}
	if len(got) != len(want) || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("appendFlag = %v, want %v", got, want)
	}
}
