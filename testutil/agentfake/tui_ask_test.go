package agentfake

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// askFake starts the TUI fake with a PreToolUse hook that records its stdin to
// the returned file and then holds until it is signalled.
func askFake(t *testing.T) (f *tuiFake, payloadFile string) {
	t.Helper()
	payloadFile = filepath.Join(t.TempDir(), "payload.json")
	settings, err := json.Marshal(map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{
		"matcher": "AskUserQuestion",
		"hooks":   []any{map[string]any{"type": "command", "command": "cat > " + payloadFile + "; exec sleep 60"}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, tuiLaunchArgs...), "--settings", string(settings))
	f = startTUIFake(t, args)
	f.waitStatus("idle", "")
	return f, payloadFile
}

var askCall = &AskCall{ToolUseID: "toolu_ask", Questions: []AskQuestion{{
	Header: "Pick", Question: "Which?", MultiSelect: true,
	Options: []AskOption{{Label: "A", Description: "first"}, {Label: "B", Description: "second"}},
}}}

func TestTUIAskQuestionRunsTheHookBusyWhileItHolds(t *testing.T) {
	f, payloadFile := askFake(t)
	f.control(Control{Action: ActionAskQuestion, Ask: askCall})
	f.waitStatus("busy", "")
	f.waitFor("the hook payload", func() bool {
		data, _ := os.ReadFile(payloadFile)
		return len(data) > 0
	})
	data, _ := os.ReadFile(payloadFile)
	var got struct {
		Event     string `json:"hook_event_name"`
		Tool      string `json:"tool_name"`
		ToolUseID string `json:"tool_use_id"`
		Input     struct {
			Questions []AskQuestion `json:"questions"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Event != "PreToolUse" || got.Tool != "AskUserQuestion" || got.ToolUseID != "toolu_ask" {
		t.Errorf("payload = %s", data)
	}
	if len(got.Input.Questions) != 1 || !got.Input.Questions[0].MultiSelect || got.Input.Questions[0].Options[1].Description != "second" {
		t.Errorf("questions = %+v", got.Input.Questions)
	}
}

func TestTUIAskQuestionEscSignalsTheHookAndEndsTheTurn(t *testing.T) {
	f, _ := askFake(t)
	f.control(Control{Action: ActionAskQuestion, Ask: askCall})
	f.waitStatus("busy", "")
	if _, err := io.WriteString(f.stdin, "\x1b"); err != nil {
		t.Fatal(err)
	}
	f.waitStatus("idle", "")
}
