package agentfake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// pidEnv is the env of a fake whose pid files live under a temp config dir,
// not the test process's own.
func pidEnv(t *testing.T) []string {
	t.Helper()
	return []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()}
}

func writePidFile(t *testing.T, env []string, pid int, sessionID, status string) {
	t.Helper()
	dir := env[0][len("CLAUDE_CONFIG_DIR="):]
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(PidFile{Pid: pid, SessionID: sessionID, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions", string(rune('0'+pid%10))+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPidStatusFor_ReturnsTheStatusOfTheAskedSession(t *testing.T) {
	env := pidEnv(t)
	writePidFile(t, env, 1, "conv-a", StatusBusy)
	writePidFile(t, env, 2, "conv-b", StatusIdle)
	writePidFile(t, env, 3, "conv-b-cleared", StatusWaiting)

	for _, c := range []struct{ id, want string }{
		{"conv-a", StatusBusy},
		{"conv-b", StatusIdle},
		{"conv-b-cleared", StatusWaiting},
	} {
		got, found := PidStatusFor(env, c.id)
		if !found || got != c.want {
			t.Errorf("PidStatusFor(%q) = %q, %v; want %q", c.id, got, found, c.want)
		}
	}
	if _, found := PidStatusFor(env, "conv-missing"); found {
		t.Error("found a pid file for a session no file names")
	}
}

func TestTurnEnded_NeedsTheReplyAndAnIdlePidFile(t *testing.T) {
	env := pidEnv(t)
	writePidFile(t, env, 1, "conv-busy", StatusBusy)
	writePidFile(t, env, 2, "conv-idle", StatusIdle)
	for _, c := range []struct {
		id      string
		replies int
		want    bool
	}{
		{"conv-busy", 1, false},
		{"conv-idle", 0, false},
		{"conv-idle", 1, true},
		{"conv-missing", 1, false},
	} {
		if err := TurnEnded(env, c.id, c.replies); (err == nil) != c.want {
			t.Errorf("TurnEnded(%q, %d) = %v; want ended %v", c.id, c.replies, err, c.want)
		}
	}
}

func TestPidStatusFor_NoSessionsDirIsNotFound(t *testing.T) {
	if _, found := PidStatusFor(pidEnv(t), "conv-a"); found {
		t.Error("found a pid file in a missing directory")
	}
}
