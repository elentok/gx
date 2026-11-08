package transcript

import (
	"path/filepath"
	"testing"
)

const (
	toolUseLine   = `{"type":"assistant","timestamp":"2026-10-11T07:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}}`
	noVerdictLine = `{"type":"user","timestamp":"2026-10-11T07:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"The server-side auto mode classifier gave no verdict (error)"}]}}`
	okResultLine  = `{"type":"user","timestamp":"2026-10-11T07:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":false,"content":"ok"}]}}`
	otherErrLine  = `{"type":"user","timestamp":"2026-10-11T07:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"exit status 1"}]}}`
)

func TestCountClassifierNoVerdict(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"three in a row", []string{replyLine, toolUseLine, noVerdictLine, toolUseLine, noVerdictLine, toolUseLine, noVerdictLine}, 3},
		{"a trailing assistant line does not break the run", []string{toolUseLine, noVerdictLine, toolUseLine}, 1},
		{"a success ends the run", []string{toolUseLine, noVerdictLine, toolUseLine, okResultLine, toolUseLine, noVerdictLine}, 1},
		{"a non-outage tail", []string{toolUseLine, noVerdictLine, toolUseLine, okResultLine}, 0},
		{"another tool error", []string{toolUseLine, otherErrLine}, 0},
		{"a person record ends the run", []string{toolUseLine, noVerdictLine, userLine}, 0},
		{"empty", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CountClassifierNoVerdict(writeTranscript(t, c.lines...))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("count = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCountClassifierNoVerdict_MissingFile(t *testing.T) {
	if n, err := CountClassifierNoVerdict(filepath.Join(t.TempDir(), "none.jsonl")); n != 0 || err != nil {
		t.Fatalf("missing file: n=%d err=%v", n, err)
	}
}
