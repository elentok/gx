package codexsession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnEnded(t *testing.T) {
	user := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"text":"hi"}]}}`
	asst := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"text":"yo"}]}}`
	done := `{"type":"event_msg","payload":{"type":"task_complete"}}`
	cases := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"complete after last message", []string{user, asst, done}, true},
		{"no task_complete", []string{user, asst}, false},
		{"new message after complete", []string{user, asst, done, user}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(c.lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := TurnEnded(path)
			if err != nil || got != c.want {
				t.Fatalf("TurnEnded = %v, %v; want %v", got, err, c.want)
			}
		})
	}
}
