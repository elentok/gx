package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePidFile(t *testing.T, dir string, startedAt time.Time, fields string) {
	t.Helper()
	data := fmt.Sprintf(`{"pid":4242,"sessionId":"conv-1","startedAt":%d%s}`, startedAt.UnixMilli(), fields)
	if err := os.WriteFile(filepath.Join(dir, "4242.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPidWatcher_IgnoresFileFromBeforeSpawn(t *testing.T) {
	dir := t.TempDir()
	spawned := time.Now()
	w := newPidWatcher(dir, 4242, spawned)

	writePidFile(t, dir, spawned.Add(-time.Hour), `,"status":"idle"`)
	w.poll()
	if s, _ := w.PidState(); s != nil {
		t.Fatalf("stale file from a reused pid read as %+v", *s)
	}
	if got := w.Status(); got.State != Starting {
		t.Fatalf("stale file: Status = %+v, want Starting", got)
	}

	// Claude's startedAt is a little after the spawn; slack covers rounding.
	writePidFile(t, dir, spawned.Add(-time.Second), `,"status":"idle"`)
	w.poll()
	if s, turn := w.PidState(); s == nil || s.Status != ClaudeIdle || turn != 1 {
		t.Fatalf("fresh file: got %v at turn %d", s, turn)
	}
}

func TestPidWatcher_IgnoresFileWithoutStartedAt(t *testing.T) {
	dir := t.TempDir()
	w := newPidWatcher(dir, 4242, time.Now())
	if err := os.WriteFile(filepath.Join(dir, "4242.json"), []byte(`{"pid":4242,"status":"idle"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	w.poll()
	if s, _ := w.PidState(); s != nil {
		t.Fatalf("file without startedAt read as %+v", *s)
	}
}

func TestPidWatcher_ReportsSessionStatus(t *testing.T) {
	dir := t.TempDir()
	spawned := time.Now()
	w := newPidWatcher(dir, 4242, spawned)
	w.poll()
	if got := w.Status(); got != (Status{State: Starting}) {
		t.Fatalf("before the file: Status = %+v, want Starting", got)
	}
	if got := w.ConversationID(); got != "" {
		t.Fatalf("before the file: ConversationID = %q, want empty", got)
	}

	cases := []struct {
		fields string
		want   Status
	}{
		{`,"status":"busy"`, Status{State: Working}},
		{`,"status":"shell"`, Status{State: Working, Background: true}},
		{`,"status":"idle"`, Status{State: Idle}},
		{`,"status":"waiting","waitingFor":"permission prompt"`, Status{State: Blocked, Blocked: BlockedPermission, Message: "permission prompt"}},
		{`,"status":"waiting","waitingFor":"input needed"`, Status{State: Blocked, Blocked: BlockedQuestion, Message: "input needed"}},
	}
	for _, c := range cases {
		writePidFile(t, dir, spawned, c.fields)
		w.poll()
		if got := w.Status(); got != c.want {
			t.Errorf("%s: Status = %+v, want %+v", c.fields, got, c.want)
		}
	}
	if got := w.ConversationID(); got != "conv-1" {
		t.Errorf("ConversationID = %q, want conv-1", got)
	}
}

func TestPidWatcher_HoldFreezesStatus(t *testing.T) {
	dir := t.TempDir()
	spawned := time.Now()
	w := newPidWatcher(dir, 4242, spawned)
	writePidFile(t, dir, spawned, `,"status":"waiting","waitingFor":"permission prompt"`)
	w.poll()
	w.Hold()
	writePidFile(t, dir, spawned, `,"status":"idle"`)
	w.poll()
	if got := w.Status(); got.State != Blocked {
		t.Fatalf("held: Status = %+v, want Blocked", got)
	}
	w.Release()
	if got := w.Status(); got.State != Idle {
		t.Fatalf("released: Status = %+v, want Idle", got)
	}
}
