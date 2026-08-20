package transcript

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func realUserTurnLine(text string) string {
	return `{"type":"user","isSidechain":false,"message":{"content":` + jsonString(text) + `}}`
}

func toolResultUserLine(toolUseID, content string) string {
	return `{"type":"user","isSidechain":false,"message":{"content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":` + jsonString(content) + `}]}}`
}

func assistantTextLine(text string) string {
	return `{"type":"assistant","isSidechain":false,"message":{"content":[{"type":"text","text":` + jsonString(text) + `}]}}`
}

func assistantToolUseLine(toolUseID, name string) string {
	return `{"type":"assistant","isSidechain":false,"message":{"content":[{"type":"tool_use","id":"` + toolUseID + `","name":"` + name + `","input":{}}]}}`
}

func assistantToolUseThenTextLine(toolUseID, name, text string) string {
	return `{"type":"assistant","isSidechain":false,"message":{"content":[{"type":"tool_use","id":"` + toolUseID + `","name":"` + name + `","input":{}},{"type":"text","text":` + jsonString(text) + `}]}}`
}

func sidechainAssistantTextLine(text string) string {
	return `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":` + jsonString(text) + `}]}}`
}

// jsonString renders s as a JSON string literal — a tiny helper so fixture
// builders above can embed arbitrary text without hand-escaping quotes.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestReadUnexecutedToolCall_MatchesLoneUnexecutedCallText(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("do the thing"),
		assistantTextLine("Agent({\n  description: \"do the thing\",\n"),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if !matched {
		t.Errorf("matched = false, want true for a lone call-shaped text block")
	}
}

func TestReadUnexecutedToolCall_RealToolUseNeverMatchesEvenWithAccompanyingText(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("do the thing"),
		assistantToolUseThenTextLine("tool-1", "Bash", "Bash({command: \"ls\"})"),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: turn has a real tool_use block")
	}
}

func TestReadUnexecutedToolCall_ToolUseElsewhereInTurnStillBlocksMatch(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("do the thing"),
		assistantToolUseLine("tool-1", "Bash"),
		toolResultUserLine("tool-1", "ok"),
		assistantTextLine("Agent({\n"),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: an earlier tool_use in the same turn must still block the match")
	}
}

func TestReadUnexecutedToolCall_OrdinaryProseNeverMatches(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("what does this function do"),
		assistantTextLine("This function reads the config file and parses it as JSON."),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: ordinary prose isn't call-shaped")
	}
}

func TestReadUnexecutedToolCall_OnlyLastTurnConsidered(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("first"),
		assistantTextLine("Agent({\n"),
		realUserTurnLine("second"),
		assistantTextLine("All done, nothing left to do."),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: an earlier turn's call-shaped text must not leak into the last turn's verdict")
	}
}

func TestReadUnexecutedToolCall_SidechainAssistantLineIgnored(t *testing.T) {
	path := writeTranscript(t,
		realUserTurnLine("do the thing"),
		sidechainAssistantTextLine("Agent({\n"),
	)

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: a subagent's sidechain content must never count as the parent turn's last assistant block")
	}
}

func TestReadUnexecutedToolCall_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonl")

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false for a missing file")
	}
}

func TestReadUnexecutedToolCall_UnreadableFile(t *testing.T) {
	path := writeTranscript(t, "not json at all", "still not json")

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false for an unreadable file")
	}
}

func TestReadUnexecutedToolCall_NoAssistantTurnYet(t *testing.T) {
	path := writeTranscript(t, realUserTurnLine("hello"))

	matched, err := ReadUnexecutedToolCall(path)
	if err != nil {
		t.Fatalf("ReadUnexecutedToolCall() error = %v", err)
	}
	if matched {
		t.Errorf("matched = true, want false: no assistant turn has landed yet")
	}
}
