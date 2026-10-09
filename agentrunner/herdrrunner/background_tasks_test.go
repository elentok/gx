package herdrrunner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/transcript"
)

func startMarker(taskID string, at time.Time) string {
	return `{"isSidechain":false,"timestamp":"` + at.Format(time.RFC3339Nano) + `","toolUseResult":{"backgroundTaskId":"` + taskID + `"}}`
}

func notification(taskID string, at time.Time) string {
	return `{"isSidechain":false,"timestamp":"` + at.Format(time.RFC3339Nano) + `","message":{"content":"<task-id>` + taskID + `</task-id>"}}`
}

func TestClaudeBackgroundTasks(t *testing.T) {
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"outstanding fresh", []string{startMarker("task-1", now.Add(-time.Minute))}, true},
		{"resolved", []string{startMarker("task-1", now.Add(-time.Minute)), notification("task-1", now)}, false},
		{"aged out", []string{startMarker("task-1", now.Add(-transcript.BackgroundTaskAgedOutCap-time.Minute))}, false},
		{"no task", []string{`{"type":"assistant","message":{}}`}, false},
		{"no transcript", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.lines != nil {
				path := transcript.PathIn(home, "/work/repo", "sess-1")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.Join(tt.lines, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			probe := herdrrunner.ClaudeBackgroundTasks(home, func() time.Time { return now })
			if got := probe(agentrunner.Session{Label: "a", SessionID: "sess-1"}); got != tt.want {
				t.Fatalf("probe = %v, want %v", got, tt.want)
			}
		})
	}
}

// A session with no agent session id yet (or a Codex one, which has no
// Claude transcript) must not match some other session's transcript.
func TestClaudeBackgroundTasks_NoSessionID(t *testing.T) {
	home := t.TempDir()
	path := transcript.PathIn(home, "/work/repo", "sess-1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(startMarker("task-1", time.Now())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if herdrrunner.ClaudeBackgroundTasks(home, time.Now)(agentrunner.Session{Label: "a"}) {
		t.Fatal("probe = true for a session without a session id")
	}
}
