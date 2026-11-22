package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// shellOutputFixture is a Claude session whose shell "b1" writes its output
// under a tasks dir; computes counts the output-file reads.
type shellOutputFixture struct {
	*readFixture
	out      string
	computes *atomic.Int32
}

func newShellOutputFixture(t *testing.T, kind, notification string) *shellOutputFixture {
	t.Helper()
	config := t.TempDir()
	src := newFakeSource()
	src.meta = Meta{ID: "s1", Harness: "claude", Dir: testCwd, ConversationID: testConv, Flow: FlowRef{Flow: tickets.FlowDesign, Key: "gx:epic/03"}}
	c := claudeChat(src, config)
	computes := &atomic.Int32{}
	c.reads.shellOutput = func(p string) (ShellOutput, error) {
		computes.Add(1)
		return readShellOutput(p)
	}
	f := &readFixture{src: src, config: config, srv: serveChat(t, src, c),
		path: filepath.Join(config, "projects", transcript.Slugify(testCwd), testConv+".jsonl")}
	out := filepath.Join(t.TempDir(), "tasks", "b1.output")
	ts := func(ago time.Duration) string { return time.Now().UTC().Add(-ago).Format(time.RFC3339Nano) }
	input := `{"command":"make","description":"build","run_in_background":true}`
	name := "Bash"
	// A Monitor's result names its task as taskId beside timeoutMs; a shell's
	// names it as backgroundTaskId.
	result := `{"backgroundTaskId":"b1"}`
	if kind == transcript.TaskKindMonitor {
		input, name = `{"command":"tail","description":"watch","timeout_ms":3600000,"persistent":false}`, "Monitor"
		result = `{"taskId":"b1","timeoutMs":3600000}`
	}
	lines := []string{
		fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"u1","name":%q,"input":%s}]}}`, ts(time.Minute), name, input),
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"u1","content":"Output is being written to: %s"}]},"toolUseResult":%s}`, ts(50*time.Second), out, result),
	}
	if notification != "" {
		body := "<task-notification>\n<task-id>b1</task-id>\n<status>completed</status>\n<summary>" + notification + "</summary>\n</task-notification>"
		lines = append(lines, fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts(10*time.Second), body))
	}
	writeLines(t, f.path, lines...)
	return &shellOutputFixture{readFixture: f, out: out, computes: computes}
}

func (f *shellOutputFixture) write(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.out, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *shellOutputFixture) tasks(t *testing.T) []wire.SessionTask {
	t.Helper()
	return f.chat(t, "/v1/sessions/chat").Tasks
}

// addRunningCall keeps the group open once the shell has finished.
func (f *shellOutputFixture) addRunningCall(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"u9","name":"Bash","input":{"command":"sleep 30","description":"wait"}}]}}`, time.Now().UTC().Add(-30*time.Second).Format(time.RFC3339Nano))
	writeLines(t, f.path, strings.TrimRight(string(data), "\n"), line)
}

func TestRead_ShellOutputShowsSizeAndLastLineAndRefreshesOnGrowth(t *testing.T) {
	t.Parallel()
	f := newShellOutputFixture(t, transcript.TaskKindShell, "")
	f.write(t, "compiling\nlinking\n")
	got := f.tasks(t)
	if len(got) != 1 || got[0].OutputBytes != 18 || got[0].LastLine != "linking" || got[0].Finished {
		t.Fatalf("row = %+v", got)
	}
	f.tasks(t)
	if n := f.computes.Load(); n != 1 {
		t.Errorf("computes after an unchanged reread = %d, want 1", n)
	}
	f.write(t, "compiling\nlinking\nbundling\n\n")
	got = f.tasks(t)
	if got[0].OutputBytes != 28 || got[0].LastLine != "bundling" {
		t.Errorf("after growth = %+v", got[0])
	}
	if n := f.computes.Load(); n != 2 {
		t.Errorf("computes after growth = %d, want 2", n)
	}
}

func TestRead_ShellOutputTrailerFinishesAShellWithNoNotification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, outcome, last string
		code                      *int
	}{
		{"success", "ok\n[exited with code 0]\n", "success", "ok", new(0)},
		{"failure", "boom\n[exited with code 2]\n", "failure", "boom", new(2)},
		{"killed", "work\n\n[killed]\n", "killed", "work", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newShellOutputFixture(t, transcript.TaskKindShell, "")
			mtime := time.Now().Add(-20 * time.Second).UTC().Truncate(time.Second)
			f.write(t, tc.body)
			if err := os.Chtimes(f.out, mtime, mtime); err != nil {
				t.Fatal(err)
			}
			f.addRunningCall(t)
			rows := f.tasks(t)
			if len(rows) != 2 {
				t.Fatalf("rows = %+v", rows)
			}
			r := rows[0]
			at, _ := time.Parse(time.RFC3339, r.FinishedAt)
			if !r.Finished || r.Outcome != tc.outcome || r.LastLine != tc.last || !at.Equal(mtime) {
				t.Errorf("row = %+v", r)
			}
			if (tc.code == nil) != (r.ExitCode == nil) || (tc.code != nil && *r.ExitCode != *tc.code) {
				t.Errorf("exit code = %v", r.ExitCode)
			}
		})
	}
}

func TestRead_ShellOutputNotificationWinsOverTrailer(t *testing.T) {
	t.Parallel()
	f := newShellOutputFixture(t, transcript.TaskKindShell, "done (exit code 0)")
	f.write(t, "ok\n[exited with code 3]\n")
	f.addRunningCall(t)
	r := f.tasks(t)[0]
	if r.Outcome != "success" || r.LastLine != "ok" {
		t.Errorf("row = %+v", r)
	}
	if r.ExitCode == nil || *r.ExitCode != 0 {
		t.Errorf("exit code = %v, want the notification's 0", r.ExitCode)
	}
}

func TestRead_ShellOutputIgnoresMonitorsMissingAndUnsafePaths(t *testing.T) {
	t.Parallel()
	f := newShellOutputFixture(t, transcript.TaskKindMonitor, "")
	f.write(t, "line\n[killed]\n")
	got := f.tasks(t)
	if len(got) != 1 || got[0].Finished || got[0].OutputBytes != 0 || f.computes.Load() != 0 {
		t.Errorf("monitor row = %+v, computes %d", got, f.computes.Load())
	}

	missing := newShellOutputFixture(t, transcript.TaskKindShell, "")
	if got := missing.tasks(t); len(got) != 1 || got[0].OutputBytes != 0 || got[0].Finished {
		t.Errorf("missing file row = %+v", got)
	}

	for _, p := range []string{"/etc/passwd", "relative/tasks/b1.output", "/x/other/b1.output", "/x/tasks/b1.txt", "/x/tasks/../tasks/b1.output"} {
		if safeShellOutputPath(p) {
			t.Errorf("%q accepted", p)
		}
	}
}

func TestWithShellOutputs_DoesNotMutateTheCachedRecords(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "b1.output")
	if err := os.WriteFile(p, []byte("hi\n[killed]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recs := []transcript.TaskRecord{{Kind: transcript.TaskKindShell, OutputPath: p}}
	got := (&Chat{}).withShellOutputs(recs)
	if got[0].Trailer == nil || got[0].LastLine != "hi" {
		t.Errorf("filled = %+v", got[0])
	}
	if recs[0].Trailer != nil || recs[0].LastLine != "" || recs[0].OutputBytes != 0 {
		t.Errorf("cached record mutated: %+v", recs[0])
	}
}
