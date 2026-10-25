package agentfake

import (
	"strings"
	"testing"
)

func TestDesignMessage_NoScriptKeepsTheDefault(t *testing.T) {
	var tu tui
	if got := tu.designMessage(DesignPromptPrefix+"epic", nil); !strings.Contains(got, "Q8") {
		t.Errorf("first /gx-design prompt = %q, want round one", got)
	}
	if got := tu.designMessage("an answer", nil); !strings.HasSuffix(got, DesignRunEnd) {
		t.Errorf("answer = %q, want the run-end message", got)
	}
	if got := tu.designMessage(DesignPromptPrefix+"epic", nil); !strings.Contains(got, "Q9") {
		t.Errorf("second /gx-design prompt = %q, want round two", got)
	}
}

func TestDesignMessage_ScriptPlaysInOrderThenFallsBack(t *testing.T) {
	var tu tui
	script := []string{DesignTurnRound, DesignTurnPlain, DesignTurnError, DesignTurnDefaulted, DesignTurnRunEnd}
	wants := []func(string) bool{
		func(m string) bool { return strings.Contains(m, "Q8") },
		func(m string) bool { return !strings.Contains(m, "Q8") && !strings.Contains(m, DesignRunEnd) },
		func(m string) bool { return strings.Contains(m, "Error:") && strings.HasSuffix(m, DesignRunEnd) },
		func(m string) bool { return strings.Contains(m, "Accept these defaults?") },
		func(m string) bool { return strings.HasSuffix(m, DesignRunEnd) },
	}
	for i, ok := range wants {
		if got := tu.designMessage("text", script); !ok(got) {
			t.Errorf("turn %d = %q", i, got)
		}
	}
	if got := tu.designMessage("text", script); !strings.HasSuffix(got, DesignRunEnd) {
		t.Errorf("turn past the script = %q, want the default run-end message", got)
	}
}
