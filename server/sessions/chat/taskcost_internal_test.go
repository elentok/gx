package chat

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// These tests pin the cost model of the task list: each per-file task reader
// runs inside a FileCache, only on a chat read, and the clock never costs a parse.

var costBase = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// readCounts counts the calls each task-file reader gets, by path. The poll runs
// on its own goroutine, so it locks.
type readCounts struct {
	mu sync.Mutex
	n  map[string]int
}

func (r *readCounts) add(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n[path]++
}

func (r *readCounts) all() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	maps.Copy(out, r.n)
	return out
}

// countReads wraps the three per-file readers of c with counters.
func countReads(c *Chat) *readCounts {
	r := &readCounts{n: map[string]int{}}
	c.reads.shellOutput = func(p string) (ShellOutput, error) { r.add(p); return readShellOutput(p) }
	c.reads.subagent = func(p string) (transcript.SubagentDetails, error) {
		r.add(p)
		return transcript.ReadSubagentFile(p)
	}
	c.reads.codexChild = func(p string) (codexChild, error) { r.add(p); return readCodexChild(p) }
	return r
}

// costEnv is a fixture whose Chat counts file reads and runs on a test clock.
type costEnv struct {
	f     *readFixture
	c     *Chat
	reads *readCounts
	nowN  atomic.Int64
	tasks string // a dir named tasks, where shell output files live
	mtime int
}

func newCostEnv(t *testing.T, f *readFixture) *costEnv {
	t.Helper()
	config := f.config
	if config == "" {
		config = t.TempDir()
	}
	e := &costEnv{f: f, c: claudeChat(f.src, config), tasks: filepath.Join(t.TempDir(), "tasks")}
	e.c.cfg.TranscriptMissTTL = time.Minute
	e.reads = countReads(e.c)
	e.at(time.Minute)
	e.c.now = func() time.Time { return time.Unix(0, e.nowN.Load()).UTC() }
	f.srv = serveChat(t, f.src, e.c)
	return e
}

func (e *costEnv) at(d time.Duration) { e.nowN.Store(costBase.Add(d).UnixNano()) }

func (e *costEnv) read(t *testing.T) wire.SessionChatResponse {
	t.Helper()
	return e.f.chat(t, "/v1/sessions/chat")
}

func (e *costEnv) output(task string) string { return filepath.Join(e.tasks, task+".output") }

// subagentFile is where the parent's transcript path puts a subagent's file.
func (e *costEnv) subagentFile(id string) string {
	return strings.TrimSuffix(e.f.path, ".jsonl") + "/subagents/agent-" + id + ".jsonl"
}

// touch gives path a later mtime than any it had, without sleeping.
func (e *costEnv) touch(t *testing.T, path string) {
	t.Helper()
	e.mtime++
	m := costBase.Add(time.Duration(e.mtime) * time.Hour)
	if err := os.Chtimes(path, m, m); err != nil {
		t.Fatal(err)
	}
}

func costTS(d time.Duration) string { return costBase.Add(d).Format(time.RFC3339Nano) }

func costUse(name, id string, at time.Duration, input string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, costTS(at), id, name, input)
}

func costResult(id string, at time.Duration, text, toolUseResult string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":%q}]},"toolUseResult":%s}`, costTS(at), id, text, toolUseResult)
}

// costFinal is a final task notification; usage is empty or the <usage> block.
func costFinal(task, usage string, at time.Duration) string {
	body := "<task-notification>\n<task-id>" + task + "</task-id>\n<status>completed</status>\n<summary>done (exit code 0)</summary>\n" + usage + "</task-notification>"
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, costTS(at), body)
}

const costUsage = "<usage><subagent_tokens>900</subagent_tokens><tool_uses>3</tool_uses></usage>\n"

// costShell launches a background shell whose output file is out.
func costShell(id, task, out string, at time.Duration) []string {
	return []string{
		costUse("Bash", id, at, `{"command":"make","description":"`+task+`","run_in_background":true}`),
		costResult(id, at, "Output is being written to: "+out, fmt.Sprintf(`{"backgroundTaskId":%q}`, task)),
	}
}

// costSubagent launches a background subagent; its file is the one next to the
// parent transcript.
func costSubagent(id, agent string, at time.Duration) []string {
	return []string{
		costUse("Agent", id, at, `{"description":"`+agent+`","prompt":"p"}`),
		costResult(id, at, "ok", fmt.Sprintf(`{"status":"async_launched","agentId":%q,"resolvedModel":"m"}`, agent)),
	}
}

func costMonitor(id, task, label string, at time.Duration) []string {
	return []string{
		costUse("Monitor", id, at, `{"command":"tail","description":"`+label+`","timeout_ms":600000}`),
		costResult(id, at, "ok", fmt.Sprintf(`{"taskId":%q,"timeoutMs":600000}`, task)),
	}
}

func costJoin(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

const costSubagentLine = `{"type":"assistant","message":{"model":"m","usage":{"input_tokens":5,"output_tokens":5},"content":[{"type":"text","text":"x"}]}}`

func wantReads(t *testing.T, e *costEnv, want map[string]int) {
	t.Helper()
	if got := e.reads.all(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reads = %v, want %v", got, want)
	}
}

func TestTaskCost_UnchangedFilesParseOnce(t *testing.T) {
	t.Parallel()
	e := newCostEnv(t, newClaudeFixture(t))
	shellFile, agentFile := e.output("b1"), e.subagentFile("ag1")
	writeLines(t, shellFile, "building")
	writeLines(t, agentFile, costSubagentLine)
	writeLines(t, e.f.path, costJoin(costShell("u1", "b1", shellFile, 0), costSubagent("a1", "ag1", time.Second))...)

	e.read(t)
	e.read(t)
	wantReads(t, e, map[string]int{shellFile: 1, agentFile: 1})

	cf := newCodexFixture(t)
	ce := newCostEnv(t, cf)
	childFile := filepath.Join(cf.src.codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-th-file.jsonl")
	writeLines(t, childFile,
		`{"type":"turn_context","payload":{"model":"child-model"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":100}}}}`)
	writeLines(t, cf.path,
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"cli_version":"0.150.0"}}`, costTS(0)),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"task_started"}}`, costTS(time.Second)),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"c1","arguments":"{\"task_name\":\"kid\"}"}}`, costTS(2*time.Second)),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":"started","id":"c1","agent_thread_id":"th-file","agent_path":"/root/x","model":"m"}}}`, costTS(3*time.Second)),
	)
	if got := ce.read(t).Tasks; len(got) != 1 || got[0].Tokens != 100 {
		t.Fatalf("codex tasks = %+v, want one subagent with 100 tokens", got)
	}
	ce.read(t)
	wantReads(t, ce, map[string]int{childFile: 1})
}

func TestTaskCost_GrownFileReparsesAlone(t *testing.T) {
	t.Parallel()
	e := newCostEnv(t, newClaudeFixture(t))
	shellFile, agentFile := e.output("b1"), e.subagentFile("ag1")
	writeLines(t, shellFile, "one")
	writeLines(t, agentFile, costSubagentLine)
	writeLines(t, e.f.path, costJoin(costShell("u1", "b1", shellFile, 0), costSubagent("a1", "ag1", time.Second))...)
	e.read(t)

	writeLines(t, shellFile, "one", "two")
	e.touch(t, shellFile)
	e.read(t)
	wantReads(t, e, map[string]int{shellFile: 2, agentFile: 1})
}

func TestTaskCost_FinishedTaskFileIsNotParsedAgain(t *testing.T) {
	t.Parallel()
	e := newCostEnv(t, newClaudeFixture(t))
	shellFile, agentFile, doneFile := e.output("b1"), e.subagentFile("ag1"), e.subagentFile("ag2")
	writeLines(t, shellFile, "building")
	writeLines(t, agentFile, costSubagentLine)
	writeLines(t, doneFile, costSubagentLine)
	running := costJoin(costShell("u1", "b1", shellFile, 0), costSubagent("a1", "ag1", time.Second))
	writeLines(t, e.f.path, running...)
	e.read(t)
	wantReads(t, e, map[string]int{shellFile: 1, agentFile: 1})

	// The shell ends with its trailer; ag1 ends with usage; ag2 starts and ends with usage.
	writeLines(t, shellFile, "building", "[exited with code 0]")
	e.touch(t, shellFile)
	writeLines(t, e.f.path, costJoin(running,
		costSubagent("a2", "ag2", 2*time.Second),
		[]string{
			costFinal("b1", "", 10*time.Second),
			costFinal("ag1", costUsage, 11*time.Second),
			costFinal("ag2", costUsage, 12*time.Second),
		})...)
	e.read(t)
	settled := e.reads.all()
	if settled[shellFile] > 2 || settled[agentFile] != 1 || settled[doneFile] != 0 {
		t.Fatalf("after the finishing read = %v, want the shell at most twice, ag1 as before, ag2 never", settled)
	}

	e.read(t)
	e.read(t)
	wantReads(t, e, settled)
}

func TestTaskCost_FiftyTasksUnchangedParseNothing(t *testing.T) {
	t.Parallel()
	e := newCostEnv(t, newClaudeFixture(t))
	var lines []string
	want := map[string]int{}
	for i := range 50 {
		task := fmt.Sprintf("b%d", i)
		out := e.output(task)
		writeLines(t, out, "working")
		want[out] = 1
		lines = append(lines, costShell(fmt.Sprintf("u%d", i), task, out, time.Duration(i)*time.Second)...)
	}
	writeLines(t, e.f.path, lines...)

	if got := e.read(t).Tasks; len(got) != 50 {
		t.Fatalf("%d tasks, want 50", len(got))
	}
	wantReads(t, e, want)
	e.read(t)
	wantReads(t, e, want)
}

func TestTaskCost_NoChatReadOpensNoTaskFile(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	e := newCostEnv(t, f)
	shellFile, agentFile := e.output("b1"), e.subagentFile("ag1")
	writeLines(t, shellFile, "building")
	writeLines(t, agentFile, costSubagentLine)
	writeLines(t, f.path, costJoin(costShell("u1", "b1", shellFile, 0), costSubagent("a1", "ag1", time.Second))...)

	e.c.cfg.ChatPollInterval = 20 * time.Millisecond
	p := e.c.NewPoll("dir", "s1")
	defer p.Close()
	p.SetActive(true)
	for range 5 {
		<-f.src.noted
	}
	if _, err := e.c.ReadSnapshot(f.path, harness.Claude, f.src.meta.Flow, "dir"); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.get(t, "/v1/sessions/transcript"); code != 200 {
		t.Fatalf("transcript route = %d", code)
	}
	wantReads(t, e, map[string]int{})
}

func TestTaskCost_ClockPastMonitorTimeoutChangesTheRowAndParsesNothing(t *testing.T) {
	t.Parallel()
	e := newCostEnv(t, newClaudeFixture(t))
	shellFile := e.output("b1")
	writeLines(t, shellFile, "building")
	// A running shell keeps the group open once the Monitor times out.
	lines := func(label string) []string {
		return costJoin(costMonitor("m1", "mon1", label, 0), costShell("u1", "b1", shellFile, time.Second))
	}
	writeLines(t, e.f.path, lines("watch-AAAA")...)
	info, err := os.Stat(e.f.path)
	if err != nil {
		t.Fatal(err)
	}

	byKind := func(rows []wire.SessionTask, kind string) wire.SessionTask {
		for _, r := range rows {
			if r.Kind == kind {
				return r
			}
		}
		t.Fatalf("no %s row in %+v", kind, rows)
		return wire.SessionTask{}
	}
	if m := byKind(e.read(t).Tasks, transcript.TaskKindMonitor); m.Finished || m.Label != "watch-AAAA" {
		t.Fatalf("before the timeout: %+v", m)
	}

	// Same length, other label, old mtime: a reparse would show the new label.
	writeLines(t, e.f.path, lines("watch-BBBB")...)
	if err := os.Chtimes(e.f.path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	e.at(11 * time.Minute)
	m := byKind(e.read(t).Tasks, transcript.TaskKindMonitor)
	if !m.Finished || m.Outcome != "" || m.NoDuration || m.Label != "watch-AAAA" {
		t.Fatalf("after the timeout: %+v", m)
	}
	start, err1 := time.Parse(time.RFC3339, m.StartedAt)
	end, err2 := time.Parse(time.RFC3339, m.FinishedAt)
	if err1 != nil || err2 != nil || !end.Equal(costBase.Add(10*time.Minute)) || end.Sub(start) != 600000*time.Millisecond {
		t.Fatalf("finish = %q (start %q), want base+10m and a 600000 ms duration", m.FinishedAt, m.StartedAt)
	}
	wantReads(t, e, map[string]int{shellFile: 1})
}

// Not parallel: it swaps the package's task-scan variable.
func TestLoader_ClaudeScansTheTranscriptOnceForTasksAndOpenCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	line := costUse("Bash", "b1", 0, `{"command":"sleep 5"}`)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scans := 0
	orig := readClaudeTasks
	readClaudeTasks = func(p string) ([]transcript.TaskRecord, []transcript.InFlightTool, error) {
		scans++
		return orig(p)
	}
	t.Cleanup(func() { readClaudeTasks = orig })

	snap, err := Loader(harness.Claude, FlowRef{}, t.TempDir(), false)(path)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 1 || len(snap.Records) != 1 || len(snap.InFlight) != 1 {
		t.Errorf("scans = %d, records = %d, open calls = %d; want 1, 1, 1", scans, len(snap.Records), len(snap.InFlight))
	}
}
