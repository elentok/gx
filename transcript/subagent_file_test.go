package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSubagentFile_DedupesTokensAndFindsTheOpenCall(t *testing.T) {
	t.Parallel()
	lines := []string{
		// Two lines of one API message: same id, same usage.
		`{"isSidechain":true,"type":"assistant","message":{"id":"m1","model":"claude-a","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":9999},"content":[{"type":"thinking","thinking":"x"}]}}`,
		`{"isSidechain":true,"type":"assistant","message":{"id":"m1","model":"claude-a","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a"}}]}}`,
		`{"isSidechain":true,"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}`,
		`{"isSidechain":true,"type":"assistant","message":{"id":"m2","model":"claude-b","usage":{"input_tokens":120,"output_tokens":28},"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"ls /private/tmp"}}]}}`,
	}
	p := filepath.Join(t.TempDir(), "agent-x.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSubagentFile(p)
	want := SubagentDetails{Model: "claude-b", Tokens: 298, ToolCalls: 2, CurrentCall: InFlightTool{Kind: InFlightShell, Label: "ls /private/tmp"}}
	if err != nil || got != want {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
}
