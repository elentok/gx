package harness

import (
	"encoding/base64"
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

// feedOSC7501 hands w every report in a recorded byte stream.
func feedOSC7501(w *PidWatcher, stream string) {
	var p ProgramStatusParser
	_, reports := p.Feed([]byte(stream))
	for _, r := range reports {
		w.Program(r)
	}
}

func TestPidWatcher_ProgramStatusOverPidFile(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString
	cases := []struct {
		name   string
		pid    string // "" leaves the pid file unwritten
		stream string
		want   Status
	}{
		{"osc before the pid file ends Starting", "", "\x1b]7501;state=working:app=claude\a", Status{State: Working}},
		{"working wins over idle", `,"status":"idle"`, "\x1b]7501;state=working:app=claude\a", Status{State: Working}},
		{"done is a finished turn", `,"status":"busy"`, "\x1b]7501;state=working:app=claude\a\x1b]7501;state=done:app=claude\a", Status{State: Idle}},
		{"idle is a cancel", `,"status":"busy"`, "\x1b]7501;state=idle:app=claude\x1b\\", Status{State: Idle, Cancelled: true}},
		{"blocked permission", `,"status":"busy"`, "\x1b]7501;state=blocked:app=claude:kind=permission:msg=" + b64([]byte("Allow Bash?")) + "\a", Status{State: Blocked, Blocked: BlockedPermission, Message: "Allow Bash?"}},
		{"blocked question", `,"status":"busy"`, "\x1b]7501;state=blocked:app=claude:kind=question:msg=" + b64([]byte("Which one?")) + "\a", Status{State: Blocked, Blocked: BlockedQuestion, Message: "Which one?"}},
		{"clear falls back to the pid file", `,"status":"shell"`, "\x1b]7501;state=working:app=claude\a\x1b]7501;state=clear:app=claude\a", Status{State: Working, Background: true}},
		{"a task report leaves the session status", `,"status":"idle"`, "\x1b]7501;state=working:app=claude:id=t1\a", Status{State: Idle}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			spawned := time.Now()
			w := newPidWatcher(dir, 4242, spawned)
			if c.pid != "" {
				writePidFile(t, dir, spawned, c.pid)
				w.poll()
			}
			feedOSC7501(w, c.stream)
			if got := w.Status(); got != c.want {
				t.Errorf("Status = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestPidWatcher_ConversationIDFollowsPidFile(t *testing.T) {
	dir := t.TempDir()
	spawned := time.Now()
	w := newPidWatcher(dir, 4242, spawned)
	for _, id := range []string{"conv-1", "conv-2"} {
		data := fmt.Sprintf(`{"pid":4242,"sessionId":%q,"startedAt":%d,"status":"idle"}`, id, spawned.UnixMilli())
		if err := os.WriteFile(filepath.Join(dir, "4242.json"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		w.poll()
		if got := w.ConversationID(); got != id {
			t.Fatalf("ConversationID = %q, want %q", got, id)
		}
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
