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
		{"done with no shell is Idle", `,"status":"idle"`, "\x1b]7501;state=done:app=claude\a", Status{State: Idle}},
		{"done with a background shell is Working +bg", `,"status":"shell"`, "\x1b]7501;state=working:app=claude\a\x1b]7501;state=done:app=claude\a", Status{State: Working, Background: true}},
		{"a cancel with a background shell stays a cancel", `,"status":"shell"`, "\x1b]7501;state=idle:app=claude\a", Status{State: Idle, Cancelled: true}},
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

// rateLimitWatcher is a watcher on conv-1, whose transcript is at the path
// returned, under the config dir that holds the sessions dir.
func rateLimitWatcher(t *testing.T, spawned time.Time) (w *PidWatcher, sessions, transcriptPath string) {
	t.Helper()
	cfg := t.TempDir()
	sessions = filepath.Join(cfg, "sessions")
	project := filepath.Join(cfg, "projects", "-repo")
	for _, d := range []string{sessions, project} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// An empty transcript is found on the first poll, before any backoff.
	transcriptPath = filepath.Join(project, "conv-1.jsonl")
	appendLine(t, transcriptPath, "")
	return newPidWatcher(sessions, 4242, spawned), sessions, transcriptPath
}

func TestPidWatcher_TranscriptSearchBacksOff(t *testing.T) {
	spawned := time.Now()
	w, sessions, tr := rateLimitWatcher(t, spawned)
	if err := os.Remove(tr); err != nil {
		t.Fatal(err)
	}
	clock := spawned
	w.now = func() time.Time { return clock }
	var searches []time.Duration
	find := w.find
	w.find = func(dir, id string) ([]string, error) {
		searches = append(searches, clock.Sub(spawned))
		return find(dir, id)
	}
	writePidFile(t, sessions, spawned, `,"status":"idle"`)
	for range 1000 {
		w.poll()
		clock = clock.Add(pidStatusPoll)
	}
	var gaps []time.Duration
	for i := 1; i < len(searches); i++ {
		gaps = append(gaps, searches[i]-searches[i-1])
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i] < gaps[i-1] {
			t.Fatalf("gaps between searches shrank: %v", gaps)
		}
	}
	if len(gaps) < 2 || gaps[len(gaps)-1] != transcriptSearchCap || gaps[len(gaps)-2] != transcriptSearchCap {
		t.Fatalf("gaps between searches didn't settle at %v: %v", transcriptSearchCap, gaps)
	}

	appendLine(t, tr, sessionLimitLine(spawned.Add(time.Second)))
	for range int(transcriptSearchCap / pidStatusPoll) {
		w.poll()
		clock = clock.Add(pidStatusPoll)
	}
	if got := w.Status(); got != rateLimited {
		t.Fatalf("transcript written late: Status = %+v, want %+v", got, rateLimited)
	}
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func sessionLimitLine(at time.Time) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"isApiErrorMessage":true,"error":"rate_limit","apiErrorStatus":429,"message":{"content":[{"type":"text","text":"You've hit your session limit · resets 11pm"}]}}`, at.UTC().Format(time.RFC3339Nano))
}

func replyLine(at time.Time) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"text","text":"Done."}]}}`, at.UTC().Format(time.RFC3339Nano))
}

var rateLimited = Status{State: RateLimited, Message: "You've hit your session limit · resets 11pm"}

func TestPidWatcher_RateLimitedHeldUntilNextWorking(t *testing.T) {
	spawned := time.Now()
	w, sessions, tr := rateLimitWatcher(t, spawned)
	set := func(fields string) {
		writePidFile(t, sessions, spawned, fields)
		w.poll()
	}
	set(`,"status":"busy"`)
	appendLine(t, tr, sessionLimitLine(spawned.Add(time.Second)))
	set(`,"status":"idle"`)
	if got := w.Status(); got != rateLimited {
		t.Fatalf("after the limit: Status = %+v, want %+v", got, rateLimited)
	}
	set(`,"status":"idle"`)
	if got := w.Status(); got != rateLimited {
		t.Fatalf("still idle: Status = %+v, want held %+v", got, rateLimited)
	}

	set(`,"status":"busy"`)
	if got := w.Status(); got.State != Working {
		t.Fatalf("next turn: Status = %+v, want Working", got)
	}
	// The new turn's user line leaves the old limit the last assistant line.
	appendLine(t, tr, `{"type":"user","message":{"content":"retry"}}`)
	set(`,"status":"idle"`)
	if got := w.Status(); got.State != Idle {
		t.Fatalf("old limit raised again: Status = %+v, want Idle", got)
	}

	set(`,"status":"busy"`)
	appendLine(t, tr, sessionLimitLine(spawned.Add(2*time.Second)))
	set(`,"status":"idle"`)
	if got := w.Status(); got != rateLimited {
		t.Fatalf("a second limit: Status = %+v, want %+v", got, rateLimited)
	}
	feedOSC7501(w, "\x1b]7501;state=done:app=claude\a")
	if got := w.Status(); got != rateLimited {
		t.Fatalf("OSC done: Status = %+v, want held %+v", got, rateLimited)
	}
	feedOSC7501(w, "\x1b]7501;state=working:app=claude\a")
	if got := w.Status(); got.State != Working {
		t.Fatalf("OSC working: Status = %+v, want Working", got)
	}
	appendLine(t, tr, replyLine(spawned.Add(3*time.Second)))
	feedOSC7501(w, "\x1b]7501;state=done:app=claude\a")
	set(`,"status":"idle"`)
	if got := w.Status(); got.State != Idle {
		t.Fatalf("successful turn: Status = %+v, want Idle", got)
	}
}

func TestPidWatcher_IgnoresRateLimitFromBeforeSpawn(t *testing.T) {
	spawned := time.Now()
	w, sessions, tr := rateLimitWatcher(t, spawned)
	appendLine(t, tr, sessionLimitLine(spawned.Add(-time.Hour)))
	writePidFile(t, sessions, spawned, `,"status":"idle"`)
	w.poll()
	if got := w.Status(); got.State != Idle {
		t.Fatalf("resumed past an old limit: Status = %+v, want Idle", got)
	}
}

func TestPidWatcher_Precedence(t *testing.T) {
	dialog := func() bool { return true }
	cases := []struct {
		name    string
		pid     string // "" leaves the pid file unwritten
		limited bool
		stream  string
		want    Status
	}{
		{"dialog before the session", "", false, "", Status{State: Blocked, Blocked: BlockedDialog}},
		{"pid file over a dialog", `,"status":"idle"`, false, "", Status{State: Idle}},
		{"OSC 7501 over a dialog", "", false, "\x1b]7501;state=done:app=claude\a", Status{State: Idle}},
		{"rate limit over a dialog", `,"status":"idle"`, true, "", rateLimited},
		{"pid file Blocked over a rate limit", `,"status":"waiting","waitingFor":"permission prompt"`, true, "", Status{State: Blocked, Blocked: BlockedPermission, Message: "permission prompt"}},
		{"OSC 7501 Blocked over a rate limit", `,"status":"idle"`, true, "\x1b]7501;state=blocked:app=claude:kind=question\a", Status{State: Blocked, Blocked: BlockedQuestion}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spawned := time.Now()
			w, sessions, tr := rateLimitWatcher(t, spawned)
			w.startup = dialog
			if c.limited {
				appendLine(t, tr, sessionLimitLine(spawned.Add(time.Second)))
			}
			if c.pid != "" {
				writePidFile(t, sessions, spawned, c.pid)
				w.poll()
			}
			feedOSC7501(w, c.stream)
			if got := w.Status(); got != c.want {
				t.Errorf("Status = %+v, want %+v", got, c.want)
			}
		})
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
