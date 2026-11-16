package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// bgChat reads the chat of the i-th session, in creation order.
func (r *running) bgChat(t *testing.T, i int) wire.SessionChatResponse {
	t.Helper()
	list := r.sessions(t)
	if i >= len(list) {
		t.Fatalf("sessions = %d, want more than %d", len(list), i)
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", list[i].ID, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	return chat
}

// bgTasks reads the first session's open Task group.
func (r *running) bgTasks(t *testing.T) []wire.SessionTask { return r.bgChat(t, 0).Tasks }

// waitBgTasks polls the first session's task list until ok holds, and returns it.
func (r *running) waitBgTasks(t *testing.T, what string, ok func([]wire.SessionTask) bool) []wire.SessionTask {
	t.Helper()
	return r.waitBgTasksOf(t, 0, what, ok)
}

func (r *running) waitBgTasksOf(t *testing.T, i int, what string, ok func([]wire.SessionTask) bool) []wire.SessionTask {
	t.Helper()
	var tasks []wire.SessionTask
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
		tasks = r.bgChat(t, i).Tasks
		return ok(tasks)
	}) {
		t.Fatalf("session %d tasks never showed %s: %+v", i, what, tasks)
	}
	return tasks
}

func countFinished(tasks []wire.SessionTask) int {
	n := 0
	for _, task := range tasks {
		if task.Finished {
			n++
		}
	}
	return n
}

func kindsOf(tasks []wire.SessionTask) []string {
	var kinds []string
	for _, task := range tasks {
		kinds = append(kinds, task.Kind)
	}
	return kinds
}

// lineTime is the timestamp of the transcript line holding substr.
func lineTime(t *testing.T, path, substr string) time.Time {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if !strings.Contains(sc.Text(), substr) {
			continue
		}
		var l struct {
			Timestamp time.Time `json:"timestamp"`
		}
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		return l.Timestamp
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("no line with %s in %s", substr, path)
	return time.Time{}
}

func wantStart(t *testing.T, task wire.SessionTask, want time.Time) {
	t.Helper()
	got, err := time.Parse(time.RFC3339, task.StartedAt)
	if err != nil || !got.Equal(want) {
		t.Fatalf("started_at = %q (%v), want %s", task.StartedAt, err, want.Format(time.RFC3339))
	}
}

func wantTaskEnd(t *testing.T, task wire.SessionTask, outcome string, exit *int, took time.Duration) {
	t.Helper()
	start, _ := time.Parse(time.RFC3339, task.StartedAt)
	end, err := time.Parse(time.RFC3339, task.FinishedAt)
	if !task.Finished || err != nil || task.Outcome != outcome || end.Sub(start) != took {
		t.Fatalf("task = %+v, want finished %s after %s", task, outcome, took)
	}
	if (exit == nil) != (task.ExitCode == nil) || (exit != nil && *exit != *task.ExitCode) {
		t.Fatalf("exit code = %v, want %v", task.ExitCode, exit)
	}
}

func TestBgProgress_Shapes(t *testing.T) {
	t.Parallel()
	exit2 := 2
	cases := []struct {
		shape string
		check func(t *testing.T, r *running, tasks []wire.SessionTask)
	}{
		{"claude", func(t *testing.T, r *running, tasks []wire.SessionTask) {
			wantKinds := []string{"subagent", "shell", "shell", "shell", "shell", "monitor", "call"}
			if !slices.Equal(kindsOf(tasks), wantKinds) || countFinished(tasks) != 3 {
				t.Fatalf("tasks = %+v, want kinds %v with 3 finished", tasks, wantKinds)
			}
			agent := tasks[0]
			if agent.Finished || agent.Label != "Survey the sessions code" || agent.Model == "" ||
				agent.Tokens != spec.BgSurveyTokens || agent.ToolCalls != spec.BgSurveyToolCalls || agent.CurrentCall != spec.BgSurveyCurrent {
				t.Fatalf("subagent = %+v", agent)
			}
			zero := 0
			wantTaskEnd(t, tasks[1], "success", &zero, 42*time.Second)
			wantTaskEnd(t, tasks[2], "failure", &exit2, 12*time.Second)
			wantTaskEnd(t, tasks[3], "killed", nil, 12*time.Second)
			if n := strings.Count(tasks[2].Command, "\n") + 1; n != spec.BgScriptLines {
				t.Fatalf("script command has %d lines, want %d", n, spec.BgScriptLines)
			}
			moved := tasks[4]
			wantStart(t, moved, lineTime(t, r.Transcripts["claude"], `"id":"`+spec.BgTimedOutUseID+`"`))
			if moved.Finished || !strings.HasPrefix(moved.Command, "python3 - <<'PY'") {
				t.Fatalf("moved call = %+v, want it running", moved)
			}
			if mon := tasks[5]; mon.Finished || mon.EventCount != spec.BgMonitorEvents {
				t.Fatalf("monitor = %+v, want it running with %d events", mon, spec.BgMonitorEvents)
			}
			if call := tasks[6]; call.Finished || call.Command != "make test" {
				t.Fatalf("foreground call = %+v, want it running", call)
			}
		}},
		{"single-task", func(t *testing.T, _ *running, tasks []wire.SessionTask) {
			if len(tasks) != 1 || tasks[0].Kind != "shell" || tasks[0].Finished ||
				strings.Count(tasks[0].Command, "\n")+1 != spec.BgScriptLines {
				t.Fatalf("tasks = %+v, want one running shell with a %d-line command", tasks, spec.BgScriptLines)
			}
		}},
		{"many", func(t *testing.T, _ *running, tasks []wire.SessionTask) {
			if len(tasks) != spec.BgManyTasks || countFinished(tasks) != spec.BgManyFinished {
				t.Fatalf("got %d tasks, %d finished, want %d and %d", len(tasks), countFinished(tasks), spec.BgManyTasks, spec.BgManyFinished)
			}
			if tasks[1].OutputBytes < 1500 {
				t.Fatalf("shell output = %d bytes, want about 2 KB", tasks[1].OutputBytes)
			}
		}},
		{"timed-out-shell", func(t *testing.T, r *running, tasks []wire.SessionTask) {
			if len(tasks) != 1 || tasks[0].Finished {
				t.Fatalf("tasks = %+v, want one running task", tasks)
			}
			wantStart(t, tasks[0], lineTime(t, r.Transcripts["timed-out-shell"], `"id":"`+spec.BgTimedOutUseID+`"`))
		}},
		{"codex", func(t *testing.T, r *running, first []wire.SessionTask) {
			// The first session keeps the item_completed version, the second is 0.145.0.
			for i, tasks := range [][]wire.SessionTask{first, r.bgChat(t, 1).Tasks} {
				wantKinds := []string{"shell", "shell", "subagent"}
				if !slices.Equal(kindsOf(tasks), wantKinds) || countFinished(tasks) != 1 {
					t.Fatalf("session %d tasks = %+v, want kinds %v with 1 finished", i, tasks, wantKinds)
				}
				done, yielded, agent := tasks[0], tasks[1], tasks[2]
				if !done.Finished || done.Command != "make build" {
					t.Fatalf("session %d done command = %+v", i, done)
				}
				if i == 0 {
					if done.ExitCode == nil || *done.ExitCode != 0 || done.Outcome != "success" || done.OutputBytes == 0 {
						t.Fatalf("item_completed command = %+v, want exit 0 with output", done)
					}
				} else if done.ExitCode != nil || done.Outcome != "" {
					t.Fatalf("0.145.0 command = %+v, want no exit code", done)
				}
				if yielded.Finished || yielded.Command != "make test-long" {
					t.Fatalf("session %d yielded command = %+v, want it running", i, yielded)
				}
				if agent.Finished || agent.Label != "review" || agent.Model != spec.BgCodexChildModel || agent.Tokens != spec.BgCodexChildTokens {
					t.Fatalf("session %d subagent = %+v, want the child's model and tokens", i, agent)
				}
			}
		}},
		{"old-codex", func(t *testing.T, r *running, tasks []wire.SessionTask) {
			chat := r.bgChat(t, 0)
			if len(tasks) != 0 || chat.Current == nil || chat.Current.Kind != "shell" || chat.Current.Label != spec.BgOldCodexLabel {
				t.Fatalf("tasks = %+v, current = %+v, want no tasks and the in-flight label", tasks, chat.Current)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "bg-progress", nil, "--shape", tc.shape)
			tc.check(t, r, r.bgTasks(t))
		})
	}
}

// TestBgProgress_BadShapeFails: a shape nobody builds is refused before the
// fixture seeds anything, with the known list.
func TestBgProgress_BadShapeFails(t *testing.T) {
	t.Parallel()
	want := "known: " + strings.Join(spec.BgProgressShapes, ", ")
	out, err := exec.Command(fixturetest.FixtureBin, "--gx", fixturetest.GxBin, "--fakeclaude", fixturetest.FakeBin,
		"--fakegh", fixturetest.FakeGHBin, "--shape", "bogus", "bg-progress").CombinedOutput()
	if err == nil || !strings.Contains(string(out), want) {
		t.Fatalf("--shape bogus: err = %v, output = %q, want %q", err, out, want)
	}
}

// A shell launch the test appends to a transcript at the current time.
func claudeLaunch(t *testing.T, path, command string) {
	t.Helper()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	use, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": at, "message": map[string]any{"id": "msg_new", "role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": "toolu_new", "name": "Bash", "input": map[string]any{"command": command, "run_in_background": true}}}}})
	res, _ := json.Marshal(map[string]any{"type": "user", "timestamp": at, "toolUseResult": map[string]any{"backgroundTaskId": "bnew00001"},
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_new", "content": "Command running in background with ID: bnew00001"}}}})
	appendTo(t, path, string(use)+"\n"+string(res)+"\n")
}

// A command cell the test appends to a rollout at the current time; it yields, so it keeps running.
func codexLaunch(t *testing.T, path, command string) {
	t.Helper()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	lit, _ := json.Marshal(command)
	items, _ := json.Marshal([]map[string]string{{"type": "input_text", "text": "Script running with cell ID 9"}})
	call, _ := json.Marshal(map[string]any{"timestamp": at, "type": "response_item", "payload": map[string]any{
		"type": "custom_tool_call", "name": "exec", "call_id": "cnew", "input": "const r = await tools.exec_command({cmd:" + string(lit) + "});"}})
	out, _ := json.Marshal(map[string]any{"timestamp": at, "type": "response_item", "payload": map[string]any{
		"type": "custom_tool_call_output", "call_id": "cnew", "output": string(items)}})
	appendTo(t, path, string(call)+"\n"+string(out)+"\n")
}

// TestBgProgress_ExitedThenResumed: an Exited session has no open group, and
// after a resume the tasks of the earlier run stay finished while a task of the
// new run runs. Both the Claude and the Codex session.
func TestBgProgress_ExitedThenResumed(t *testing.T) {
	t.Parallel()
	r := startWith(t, "bg-progress", nil, "--shape", "exited")
	list := r.sessions(t)
	if len(list) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list))
	}
	launches := []struct {
		key    string
		launch func(*testing.T, string, string)
	}{{spec.BgKeyExitedClaude, claudeLaunch}, {spec.BgKeyExitedCodex, codexLaunch}}
	for i, tc := range launches {
		if list[i].Status.State != wire.SessionExited {
			t.Fatalf("session %d state = %s, want Exited", i, list[i].Status.State)
		}
		if tasks := r.bgChat(t, i).Tasks; len(tasks) != 0 {
			t.Fatalf("exited session %d tasks = %+v, want the group cleared", i, tasks)
		}
		r.Gx(t, "agents", "resume", list[i].ID)
		// The old tasks are finished, or they would hold the new one's group.
		tasks := []wire.SessionTask(nil)
		if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
			if cur := r.sessions(t)[i]; cur.Status.State == wire.SessionExited {
				return false
			}
			if tasks = r.bgChat(t, i).Tasks; len(tasks) > 0 {
				return true
			}
			tc.launch(t, r.Transcripts[tc.key], "make again")
			return false
		}) {
			t.Fatalf("session %d never showed the new task", i)
		}
		if len(tasks) != 1 || tasks[0].Finished || tasks[0].Command != "make again" {
			t.Fatalf("resumed session %d tasks = %+v, want only the new running command", i, tasks)
		}
	}
}

// TestBgProgress_NoEnd: tasks with no end line are finished by the reader's
// rules, and the Monitor row alone changes when its timeout passes.
func TestBgProgress_NoEnd(t *testing.T) {
	t.Parallel()
	r := startWith(t, "bg-progress", nil, "--shape", "no-end")
	list := r.sessions(t)
	// The list is ordered by run start, and the Claude session is the older one.
	codexAt, claudeAt := 1, 0
	if len(list) != 2 || list[codexAt].Harness != "codex" || list[claudeAt].Harness == "codex" {
		t.Fatalf("sessions = %+v, want one Claude and one Codex session", list)
	}
	if cx := r.bgChat(t, codexAt).Tasks; len(cx) != 1 || cx[0].Finished || cx[0].Command != "make serve" {
		t.Fatalf("codex tasks = %+v, want the yielded cell running", cx)
	}
	const (
		oldShell, keep, call, trailer, monitor = 0, 1, 2, 3, 4
	)
	before := r.bgChat(t, claudeAt).Tasks
	if !slices.Equal(kindsOf(before), []string{"shell", "shell", "call", "shell", "monitor"}) {
		t.Fatalf("claude tasks = %+v", before)
	}
	if mon := before[monitor]; mon.Finished || mon.EventCount != 1 {
		t.Fatalf("monitor before its timeout = %+v, want it running with one event", mon)
	}
	if old := before[oldShell]; !old.Finished || !old.NoDuration || old.Outcome != "" || old.FinishedAt != "" {
		t.Fatalf("aged-out shell = %+v, want finished with no outcome and no duration", old)
	}
	if k := before[keep]; k.Finished {
		t.Fatalf("anchor shell = %+v, want it running", k)
	}
	// 25 minutes ago to the turn end 15 minutes ago.
	wantTaskEnd(t, before[call], "", nil, 10*time.Minute)
	exit3 := 3
	// The output file's mtime is 2 minutes ago; the shell started 8 minutes ago.
	wantTaskEnd(t, before[trailer], "failure", &exit3, 6*time.Minute)

	after := r.waitBgTasksOf(t, claudeAt, "the finished Monitor", func(tasks []wire.SessionTask) bool {
		return len(tasks) == len(before) && tasks[monitor].Finished
	})
	wantTaskEnd(t, after[monitor], "", nil, spec.BgNoEndMonitorTimeout)
	if after[monitor].EventCount != 1 {
		t.Fatalf("monitor after its timeout = %+v, want its one event and no more", after[monitor])
	}
	for i := range before {
		if i != monitor && !reflect.DeepEqual(before[i], after[i]) {
			t.Fatalf("row %d changed: %+v -> %+v", i, before[i], after[i])
		}
	}
}

func TestBgProgress_GrowingOutput(t *testing.T) {
	t.Parallel()
	r := startWith(t, "bg-progress", nil, "--shape", "growing-output")
	path := r.TaskFiles[spec.BgGrowingKey]
	first := r.waitBgTasks(t, "the 3-line output", func(tasks []wire.SessionTask) bool {
		return len(tasks) == 1 && tasks[0].LastLine == "three"
	})
	if first[0].Finished || first[0].OutputBytes != int64(len("one\ntwo\nthree\n")) {
		t.Fatalf("first read = %+v", first[0])
	}
	appendTo(t, path, "four\nfive\n")
	second := r.waitBgTasks(t, "the grown output", func(tasks []wire.SessionTask) bool {
		return len(tasks) == 1 && tasks[0].LastLine == "five"
	})
	if second[0].Finished || second[0].OutputBytes <= first[0].OutputBytes {
		t.Fatalf("second read = %+v, want a larger output", second[0])
	}
	appendTo(t, path, "\n[exited with code 0]\n")
	// The trailer ends the only task, so the open group is gone.
	r.waitBgTasks(t, "no open tasks after the trailer", func(tasks []wire.SessionTask) bool { return len(tasks) == 0 })
}

func TestBgProgress_SubagentWithInnerTask(t *testing.T) {
	t.Parallel()
	r := startWith(t, "bg-progress", nil, "--shape", "subagent-with-inner-task")
	tasks := r.bgTasks(t)
	if len(tasks) != 1 || tasks[0].Kind != "subagent" || tasks[0].Finished {
		t.Fatalf("tasks = %+v, want one running subagent and no inner shell", tasks)
	}
	appendTo(t, r.Transcripts["subagent-with-inner-task"], `{"type":"queue-operation","operation":"enqueue","timestamp":"`+
		time.Now().UTC().Format(time.RFC3339Nano)+`","content":"<task-notification>\n<task-id>`+spec.BgInnerAgentID+
		`</task-id>\n<status>completed</status>\n<summary>Agent done</summary>\n<usage>\n<subagent_tokens>900</subagent_tokens>\n<tool_uses>1</tool_uses>\n</usage>\n</task-notification>"}`+"\n")
	r.waitBgTasks(t, "no open tasks after the final notification", func(tasks []wire.SessionTask) bool { return len(tasks) == 0 })
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
