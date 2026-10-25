package transcript

import (
	"path/filepath"
	"testing"
)

const turnDurationLine = `{"type":"system","subtype":"turn_duration","durationMs":100}`

func TestTurnEnded(t *testing.T) {
	for name, tc := range map[string]struct {
		lines []string
		want  bool
	}{
		"turn_duration last": {[]string{realUserTurnLine("go"), assistantTextLine("done"), `{"type":"system","subtype":"stop_hook_summary"}`, turnDurationLine}, true},
		"bookkeeping after":  {[]string{assistantTextLine("done"), turnDurationLine, `{"type":"last-prompt"}`, `{"type":"ai-title"}`}, true},
		"final message only": {[]string{realUserTurnLine("go"), assistantTextLine("done")}, false},
		"next turn started":  {[]string{assistantTextLine("done"), turnDurationLine, realUserTurnLine("again")}, false},
		"older turn's end":   {[]string{turnDurationLine, assistantToolUseLine("t1", "Bash")}, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := TurnEnded(writeTranscript(t, tc.lines...))
			if err != nil || got != tc.want {
				t.Fatalf("TurnEnded = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestTurnEnded_MissingFile(t *testing.T) {
	got, err := TurnEnded(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil || got {
		t.Fatalf("TurnEnded = %v, %v; want false, nil", got, err)
	}
}
