package codexsession

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

var tasksT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func at(sec int) string { return tasksT0.Add(time.Duration(sec) * time.Second).Format(time.RFC3339) }

func execLine(sec int, id, input string) string {
	in, _ := json.Marshal(input)
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":%q,"input":%s}}`, at(sec), id, in)
}

// cellOut is a custom_tool_call_output: a JSON string holding input_text items.
func cellOut(sec int, id string, texts ...string) string {
	var items []map[string]string
	for _, s := range texts {
		items = append(items, map[string]string{"type": "input_text", "text": s})
	}
	b, _ := json.Marshal(items)
	s, _ := json.Marshal(string(b))
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":%q,"output":%s}}`, at(sec), id, s)
}

func waitCall(sec int, id string, cell int) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"wait","call_id":%q,"arguments":"{\"cell_id\":%d}"}}`, at(sec), id, cell)
}

func waitOut(sec int, id, text string) string {
	s, _ := json.Marshal(text)
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call_output","call_id":%q,"output":%s}}`, at(sec), id, s)
}

func readTasks(t *testing.T, lines ...string) []transcript.TaskRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTasks(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantEnd(t *testing.T, r transcript.TaskRecord, sec int, outcome string, code *int) {
	t.Helper()
	if r.End == nil {
		t.Fatalf("%s: no end", r.Label)
	}
	if !r.End.At.Equal(tasksT0.Add(time.Duration(sec)*time.Second)) || r.End.Outcome != outcome {
		t.Errorf("%s: end = %+v, want %ds %q", r.Label, *r.End, sec, outcome)
	}
	switch {
	case code == nil && r.ExitCode != nil, code != nil && (r.ExitCode == nil || *r.ExitCode != *code):
		t.Errorf("%s: exit code = %v, want %v", r.Label, r.ExitCode, code)
	}
}

func intp(n int) *int { return &n }

func TestReadTasks_CellsAndTheirEnds(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 80)
	got := readTasks(t,
		execLine(1, "a", `const r = await tools.exec_command({cmd:"echo a"}); text(JSON.stringify(r));`),
		cellOut(2, "a", "Script completed", `{"exit_code":3,"output":"a"}`),
		execLine(3, "b", `await tools.exec_command({"cmd":"echo b"}); await tools.exec_command({"cmd":"echo c"});`),
		cellOut(4, "b", "Script completed", `{"exit_code":0}`),
		execLine(5, "c", `await tools.exec_command({cmd:"false"})`),
		cellOut(6, "c", "Script failed"),
		execLine(7, "d", `await tools.exec_command({cmd:"sleep 1"})`),
		cellOut(8, "d", "Script terminated"),
		execLine(9, "e", `tools.exec_command({cmd:`+fmt.Sprintf("%q", long)+`})`),
		execLine(10, "f", "await tools.apply_patch({})"),
		execLine(11, "g", "const cmd = build();\nawait tools.exec_command({cmd})"),
	)
	if len(got) != 6 {
		t.Fatalf("got %d records, want 6: %+v", len(got), got)
	}
	if got[0].Kind != transcript.TaskKindShell || got[0].Label != "echo a" || got[0].Command != "echo a" || !got[0].StartedAt.Equal(tasksT0.Add(time.Second)) {
		t.Errorf("first = %+v", got[0])
	}
	wantEnd(t, got[0], 2, transcript.TaskOutcomeFailure, intp(3))
	wantEnd(t, got[1], 4, "", nil) // two commands: the exit code is not one command's
	wantEnd(t, got[2], 6, transcript.TaskOutcomeFailure, nil)
	wantEnd(t, got[3], 8, transcript.TaskOutcomeKilled, nil)
	if got[4].End != nil || got[4].Command != long || got[4].Label != long[:60]+"…" {
		t.Errorf("long unfinished = %+v", got[4])
	}
	if got[5].Command != "const cmd = build();" || got[5].End != nil {
		t.Errorf("script-built command = %+v", got[5])
	}
}

func TestReadTasks_YieldedCellEndsAtItsWaitOutput(t *testing.T) {
	t.Parallel()
	got := readTasks(t,
		execLine(1, "a", `tools.exec_command({cmd:"slow a"})`),
		cellOut(2, "a", "Script running with cell ID 7"),
		execLine(3, "b", `tools.exec_command({cmd:"slow b"})`),
		cellOut(4, "b", "Script running with cell ID 8"),
		waitCall(5, "w1", 8),
		waitOut(6, "w1", "Script running with cell ID 8"),
		waitCall(7, "w2", 8),
		waitOut(9, "w2", "Script completed\n..."),
		waitCall(10, "w3", 7),
		waitOut(12, "w3", "Script failed"),
	)
	wantEnd(t, got[0], 12, transcript.TaskOutcomeFailure, nil)
	wantEnd(t, got[1], 9, "", nil)
}

func TestReadTasks_StillRunningYieldedCellHasNoEnd(t *testing.T) {
	t.Parallel()
	got := readTasks(t,
		execLine(1, "a", `tools.exec_command({cmd:"slow"})`),
		cellOut(2, "a", "Script running with cell ID 7"),
		waitCall(3, "w1", 7),
		waitOut(4, "w1", "Script running with cell ID 7"),
		`{"timestamp":"`+at(5)+`","type":"event_msg","payload":{"type":"task_complete"}}`,
	)
	if len(got) != 1 || got[0].End != nil {
		t.Errorf("got %+v, want one row with no end", got)
	}
}

func TestReadTasks_SessionCellEndsAtTheWriteStdinPoll(t *testing.T) {
	t.Parallel()
	poll := func(sec int, id string, session int) string {
		return execLine(sec, id, fmt.Sprintf(`text(JSON.stringify(await tools.write_stdin({session_id:%d, chars:""})))`, session))
	}
	got := readTasks(t,
		execLine(1, "a", `tools.exec_command({cmd:"server a"})`),
		cellOut(2, "a", "Script completed", `{"session_id":65578,"output":"up"}`),
		execLine(3, "b", `tools.exec_command({cmd:"server b"})`),
		cellOut(4, "b", "Script completed", `{"session_id":65579}`),
		poll(5, "p1", 65578),
		cellOut(6, "p1", "Script completed", `{"session_id":65578}`), // still alive
		poll(7, "p2", 65579),
		cellOut(8, "p2", "Script completed", `{"exit_code":0}`),
		execLine(9, "p3", `await Promise.all([tools.exec_command({cmd:"x"}), tools.write_stdin({session_id:65578,chars:"\u0003"})])`),
		poll(10, "p4", 65578),
		cellOut(11, "p4", "Script completed", `{"exit_code":130}`),
	)
	if len(got) != 3 {
		t.Fatalf("got %d records, want 3 (the Promise.all cell is a command too)", len(got))
	}
	wantEnd(t, got[1], 8, transcript.TaskOutcomeSuccess, intp(0))
	wantEnd(t, got[0], 11, transcript.TaskOutcomeFailure, intp(130))
}

// commandItem is an item_completed / CommandExecution line; startedSec is the
// item's own start, which can differ from the call line's time.
func commandItem(sec, startedSec int, processID, command string, code int, output string) string {
	argv, _ := json.Marshal([]string{"/bin/zsh", "-lc", command})
	out, _ := json.Marshal(output)
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"exec-1","process_id":%q,"command":%s,"status":"completed","exit_code":%d,"duration":{"secs":0,"nanos":5542},"aggregated_output":%s},"started_at_ms":%d,"completed_at_ms":%d}}`,
		at(sec), processID, argv, code, out, tasksT0.Add(time.Duration(startedSec)*time.Second).UnixMilli(), tasksT0.Add(time.Duration(sec)*time.Second).UnixMilli())
}

func TestReadTasks_ItemCompletedEndsACommandRow(t *testing.T) {
	t.Parallel()
	got := readTasks(t,
		execLine(1, "a", `tools.exec_command({cmd:"make"})`),
		cellOut(2, "a", "Script running with cell ID 7"),
		commandItem(60, 2, "111", "make", 2, "building\nboom\n"),
		waitCall(90, "w1", 7),
		waitOut(91, "w1", "Script completed"),
	)
	wantEnd(t, got[0], 60, transcript.TaskOutcomeFailure, intp(2))
	if got[0].OutputBytes != int64(len("building\nboom\n")) || got[0].LastLine != "boom" {
		t.Errorf("output = %d bytes, last line %q", got[0].OutputBytes, got[0].LastLine)
	}
}

func TestReadTasks_ItemCompletedMatching(t *testing.T) {
	t.Parallel()
	t.Run("process id beats command text", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t,
			execLine(1, "a", `tools.exec_command({cmd:"srv"})`),
			cellOut(2, "a", "Script completed", `{"session_id":42}`),
			execLine(3, "b", `tools.exec_command({cmd:"srv"})`),
			commandItem(10, 2, "42", "srv", 0, "ok\n"),
		)
		wantEnd(t, got[0], 10, transcript.TaskOutcomeSuccess, intp(0))
		if got[1].End != nil {
			t.Errorf("second row ended: %+v", got[1].End)
		}
	})
	t.Run("latest open row starting by the item start wins", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t,
			execLine(1, "a", `tools.exec_command({cmd:"go test"})`),
			execLine(5, "b", `tools.exec_command({cmd:"go test"})`),
			execLine(30, "c", `tools.exec_command({cmd:"go test"})`),
			commandItem(40, 5, "9", "go test", 0, ""),
		)
		if got[0].End != nil || got[2].End != nil {
			t.Errorf("wrong rows ended: %+v / %+v", got[0].End, got[2].End)
		}
		wantEnd(t, got[1], 40, transcript.TaskOutcomeSuccess, intp(0))
	})
	t.Run("ended rows, other commands and multi-command scripts are not matched", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t,
			execLine(1, "a", `tools.exec_command({cmd:"one"})`),
			cellOut(2, "a", "Script completed"),
			execLine(3, "b", `tools.exec_command({cmd:"two"}); tools.exec_command({cmd:"three"})`),
			commandItem(10, 3, "1", "one", 7, ""),
			commandItem(11, 3, "2", "two", 7, ""),
			commandItem(12, 3, "3", "nothing", 7, ""),
		)
		wantEnd(t, got[0], 2, "", nil)
		if got[1].End != nil {
			t.Errorf("multi-command row ended: %+v", got[1].End)
		}
	})
}

func subagentLines() []string {
	spawn := fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"sp1","arguments":"{\"task_name\":\"review\",\"message\":\"enc\"}"}}`, at(1))
	return []string{spawn, waitOut(2, "sp1", `{"task_name":"/root/review"}`)}
}

func subActivity(sec int, kind, id, thread string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":%q,"id":%q,"agent_thread_id":%q,"agent_path":"/root/review","model":"gpt-x"}}}`, at(sec), kind, id, thread)
}

func agentWaitCall(sec int, id, name string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":%q,"call_id":%q,"arguments":"{}"}}`, at(sec), name, id)
}

func TestReadTasks_SubagentRows(t *testing.T) {
	t.Parallel()
	started := subActivity(3, "started", "sp1", "th1")
	t.Run("label, thread and model; the call output ends nothing", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t, append(subagentLines(), started)...)
		r := got[0]
		if len(got) != 1 || r.Kind != transcript.TaskKindSubagent || r.Label != "review" || r.ID != "th1" || r.Model != "gpt-x" || r.End != nil || !r.StartedAt.Equal(tasksT0.Add(time.Second)) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("without a started event only label and time", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t, subagentLines()...)
		if got[0].ID != "" || got[0].Model != "" || got[0].Label != "review" {
			t.Errorf("got %+v", got[0])
		}
	})
	t.Run("completed event ends it", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t, append(subagentLines(), started, subActivity(50, "completed", "subagent-completed-x", "th1"))...)
		wantEnd(t, got[0], 50, "", nil)
	})
	t.Run("wait_agent naming it completed ends it, a plain wait does not", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t, append(subagentLines(), started,
			agentWaitCall(10, "w1", "wait_agent"), waitOut(11, "w1", `{"message":"Wait completed.","timed_out":false}`),
			agentWaitCall(20, "w2", "wait_agent"), waitOut(21, "w2", `{"status":{"th1":{"completed":"done"}}}`),
			subActivity(50, "completed", "subagent-completed-x", "th1"),
		)...)
		wantEnd(t, got[0], 21, "", nil)
		plain := readTasks(t, append(subagentLines(), started,
			agentWaitCall(10, "w1", "wait_agent"), waitOut(11, "w1", `{"message":"Wait completed.","timed_out":false}`))...)
		if plain[0].End != nil {
			t.Errorf("plain wait ended it: %+v", plain[0].End)
		}
	})
	t.Run("list_agents names it by path", func(t *testing.T) {
		t.Parallel()
		got := readTasks(t, append(subagentLines(), started,
			agentWaitCall(10, "l1", "list_agents"),
			waitOut(12, "l1", `{"agents":[{"agent_name":"/root/review","agent_status":{"completed":"x"}}]}`))...)
		wantEnd(t, got[0], 12, "", nil)
	})
}

func TestReadTasks_MissingFile(t *testing.T) {
	t.Parallel()
	got, err := ReadTasks(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v, want nothing", got, err)
	}
}
