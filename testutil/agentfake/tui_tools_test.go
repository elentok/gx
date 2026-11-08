package agentfake

import (
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestTUIClaude_ToolUseControlLeavesCallsInFlightAndBusy(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")

	f.control(Control{Action: ActionToolUse, ToolCalls: []ToolCall{
		{Name: "Bash", Input: map[string]any{"command": "make test"}},
		{Name: "Grep", Input: map[string]any{"pattern": "needle"}},
	}})
	f.waitStatus(StatusBusy, "")

	got, err := transcript.ReadInFlightTools(transcript.PathIn(f.home, f.pidFile()["cwd"].(string), "sess-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != transcript.InFlightSearch || got[1].Kind != transcript.InFlightShell {
		t.Fatalf("in flight = %+v, want the search then the shell, newest first", got)
	}
}
