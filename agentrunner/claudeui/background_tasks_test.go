package claudeui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/transcript"
)

func startMarker(taskID string, at time.Time) string {
	return `{"isSidechain":false,"timestamp":"` + at.Format(time.RFC3339Nano) + `","toolUseResult":{"backgroundTaskId":"` + taskID + `"}}`
}

func notification(taskID string, at time.Time) string {
	return `{"isSidechain":false,"type":"user","timestamp":"` + at.Format(time.RFC3339Nano) + `","message":{"content":"<task-notification><task-id>` + taskID + `</task-id></task-notification>"}}`
}

func TestBackgroundTasks(t *testing.T) {
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
			probe := claudeui.BackgroundTasks(home, func() time.Time { return now })
			if got := probe(agentrunner.Session{Label: "a", SessionID: "sess-1"}); got != tt.want {
				t.Fatalf("probe = %v, want %v", got, tt.want)
			}
		})
	}
}

func writeTranscript(t *testing.T, home, cwd, id, line string) string {
	t.Helper()
	path := transcript.PathIn(home, cwd, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The second probe must reuse the first probe's path: a duplicate transcript
// makes a fresh search ambiguous (and so false).
func TestBackgroundTasks_CachesTranscriptPath(t *testing.T) {
	now := time.Now()
	home := t.TempDir()
	writeTranscript(t, home, "/work/repo", "sess-1", startMarker("task-1", now))
	probe := claudeui.BackgroundTasks(home, func() time.Time { return now })
	s := agentrunner.Session{Label: "a", SessionID: "sess-1"}
	if !probe(s) {
		t.Fatal("first probe = false, want true")
	}
	writeTranscript(t, home, "/work/other", "sess-1", startMarker("task-1", now))
	if !probe(s) {
		t.Fatal("second probe = false, want the cached path to be reused")
	}
}

// A moved transcript drops the cached path and is found again.
func TestBackgroundTasks_RefindsMovedTranscript(t *testing.T) {
	now := time.Now()
	home := t.TempDir()
	old := writeTranscript(t, home, "/work/repo", "sess-1", startMarker("task-1", now))
	probe := claudeui.BackgroundTasks(home, func() time.Time { return now })
	s := agentrunner.Session{Label: "a", SessionID: "sess-1"}
	if !probe(s) {
		t.Fatal("first probe = false, want true")
	}
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, home, "/work/moved", "sess-1", startMarker("task-1", now))
	if !probe(s) {
		t.Fatal("probe after move = false, want true")
	}
}

// A session with no agent session id yet (or a Codex one, which has no
// Claude transcript) must not match some other session's transcript.
func TestBackgroundTasks_NoSessionID(t *testing.T) {
	home := t.TempDir()
	path := transcript.PathIn(home, "/work/repo", "sess-1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(startMarker("task-1", time.Now())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if claudeui.BackgroundTasks(home, time.Now)(agentrunner.Session{Label: "a"}) {
		t.Fatal("probe = true for a session without a session id")
	}
}
