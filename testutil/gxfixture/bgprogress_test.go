package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// bgTasks reads the first session's open Task group.
func (r *running) bgTasks(t *testing.T) []wire.SessionTask {
	t.Helper()
	list := r.sessions(t)
	if len(list) != 1 {
		t.Fatalf("sessions = %d, want 1", len(list))
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", list[0].ID, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	return chat.Tasks
}

// waitBgTasks polls the task list until ok holds, and returns it.
func (r *running) waitBgTasks(t *testing.T, what string, ok func([]wire.SessionTask) bool) []wire.SessionTask {
	t.Helper()
	var tasks []wire.SessionTask
	if !testutil.Poll(15*time.Second, 100*time.Millisecond, func() bool {
		tasks = r.bgTasks(t)
		return ok(tasks)
	}) {
		t.Fatalf("tasks never showed %s: %+v", what, tasks)
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
	}
	for _, tc := range cases {
		t.Run(tc.shape, func(t *testing.T) {
			t.Parallel()
			r := startWith(t, "bg-progress", nil, "--shape", tc.shape)
			tc.check(t, r, r.bgTasks(t))
		})
	}
}

// TestBgProgress_BadShapeFails: a shape the next ticket builds, and one nobody
// does, are both refused before the fixture seeds anything.
func TestBgProgress_BadShapeFails(t *testing.T) {
	t.Parallel()
	for shape, want := range map[string]string{
		"codex": "shape not built yet", "old-codex": "shape not built yet", "exited": "shape not built yet", "no-end": "shape not built yet",
		"bogus": "known: " + strings.Join(spec.BgProgressShapes, ", "),
	} {
		out, err := exec.Command(fixturetest.FixtureBin, "--gx", fixturetest.GxBin, "--fakeclaude", fixturetest.FakeBin,
			"--fakegh", fixturetest.FakeGHBin, "--shape", shape, "bg-progress").CombinedOutput()
		if err == nil || !strings.Contains(string(out), want) {
			t.Fatalf("--shape %s: err = %v, output = %q, want %q", shape, err, out, want)
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
