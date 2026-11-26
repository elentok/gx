package transcript

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func namedUseLine(name, id, ts, input string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"m-%s","content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, ts, id, id, name, input)
}

func eventLine(taskID, ts string) string {
	body := "<task-notification>\n<task-id>" + taskID + "</task-id>\n<summary>ev</summary>\n<event>x</event>\n</task-notification>"
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts, body)
}

const monitorInput = `{"command":"tail -f log","description":"watch log","timeout_ms":60000,"persistent":false}`

func monitorLines(extra ...string) []string {
	return append([]string{
		namedUseLine("Monitor", "m1", t0, monitorInput),
		plainResultLine("m1", t1, false, "started", `{"taskId":"mon1","timeoutMs":60000,"persistent":false}`),
	}, extra...)
}

func TestReadTaskRecords_MonitorTimeoutAndEvents(t *testing.T) {
	t.Parallel()
	recs := readRecords(t, monitorLines(
		eventLine("mon1", "2026-05-01T12:00:10Z"),
		eventLine("mon1", "2026-05-01T12:00:20Z"),
		eventLine("other", "2026-05-01T12:00:25Z"),
		eventLine("mon1", "2026-05-01T12:01:01.004Z"), // expiry notice, after start+timeout
	)...)
	r := recs[0]
	if r.Kind != TaskKindMonitor || r.Label != "watch log" || r.Command != "tail -f log" || r.Timeout != 60*time.Second {
		t.Fatalf("rec = %+v", r)
	}
	if r.EventCount != 2 || !r.LastEventAt.Equal(mustParseTime("2026-05-01T12:00:20Z")) || r.End != nil || r.OutputBytes != 0 {
		t.Errorf("events = %d at %v, end %+v", r.EventCount, r.LastEventAt, r.End)
	}
}

func TestReadTaskRecords_MonitorEnds(t *testing.T) {
	t.Parallel()
	stop := plainResultLine("ts", t2, false, "stopped", `{"task_id":"mon1","task_type":"monitor"}`)
	r := readRecords(t, monitorLines(stop)...)[0]
	if r.End == nil || r.End.Outcome != TaskOutcomeKilled || r.End.Source != TaskEndTaskStop || !r.End.At.Equal(mustParseTime(t2)) {
		t.Errorf("taskstop end = %+v", r.End)
	}
	r = readRecords(t, monitorLines(enqueueLine("mon1", "failed", "script failed", t2))...)[0]
	if r.End == nil || r.End.Outcome != TaskOutcomeFailure || r.End.Source != TaskEndNotification {
		t.Errorf("notification end = %+v", r.End)
	}
}

const agentInput = `{"description":"explore","model":"opus","prompt":"look around"}`

func asyncAgentLines(extra ...string) []string {
	return append([]string{
		namedUseLine("Agent", "a1", t0, agentInput),
		plainResultLine("a1", t1, false, "launched", `{"isAsync":true,"status":"async_launched","agentId":"ag1","description":"explore","resolvedModel":"opus-5","outputFile":"/x/tasks/ag1.output"}`),
	}, extra...)
}

func TestReadTaskRecords_AsyncSubagent(t *testing.T) {
	t.Parallel()
	interim := strings.Replace(enqueueLine("ag1", "completed", "is waiting on its own background work", t1), "</task-notification>", "<note>"+interimNotificationNote+"</note></task-notification>", 1)
	final := strings.Replace(enqueueLine("ag1", "completed", "done", t2), "</task-notification>", "<usage><subagent_tokens>1234</subagent_tokens><tool_uses>7</tool_uses></usage></task-notification>", 1)
	r := readRecords(t, asyncAgentLines(interim)...)[0]
	if r.Kind != TaskKindSubagent || r.ID != "ag1" || r.Label != "explore" || r.Model != "opus-5" || r.OutputPath != "/x/tasks/ag1.output" || !r.StartedAt.Equal(mustParseTime(t0)) {
		t.Fatalf("rec = %+v", r)
	}
	if r.Command != "" {
		t.Errorf("the subagent prompt reached Command: %q", r.Command)
	}
	if r.CallID != "a1" {
		t.Errorf("CallID = %q, want a1", r.CallID)
	}
	if r.End != nil {
		t.Errorf("interim ended it: %+v", r.End)
	}
	r = readRecords(t, asyncAgentLines(turnEndLine(t1), interim, final)...)[0]
	if r.End == nil || r.End.Outcome != TaskOutcomeSuccess || !r.End.At.Equal(mustParseTime(t2)) || r.Tokens != 1234 || r.ToolCalls != 7 || !r.TurnEnd.IsZero() {
		t.Errorf("final = %+v", r)
	}
	r = readRecords(t, asyncAgentLines(enqueueLine("ag1", "failed", "boom", t2))...)[0]
	if r.End == nil || r.End.Outcome != TaskOutcomeFailure {
		t.Errorf("failed end = %+v", r.End)
	}
	stop := plainResultLine("ts", t2, false, "stopped", `{"task_id":"ag1","task_type":"local_agent"}`)
	r = readRecords(t, asyncAgentLines(stop)...)[0]
	if r.End == nil || r.End.Outcome != TaskOutcomeKilled {
		t.Errorf("taskstop end = %+v", r.End)
	}
}

func TestReadTaskRecords_AgentCallCarriesPromptAndSubagentType(t *testing.T) {
	t.Parallel()
	input := `{"description":"explore","prompt":"look around","subagent_type":"Explore"}`
	r := readRecords(t, namedUseLine("Agent", "a1", t0, input))[0]
	if r.Prompt != "look around" || r.SubagentType != "Explore" {
		t.Errorf("Agent: prompt %q type %q", r.Prompt, r.SubagentType)
	}
	r = readRecords(t, namedUseLine("Task", "a2", t0, input))[0]
	if r.Prompt != "look around" || r.SubagentType != "Explore" {
		t.Errorf("Task: prompt %q type %q", r.Prompt, r.SubagentType)
	}
	r = readRecords(t, namedUseLine("Bash", "b1", t0, `{"command":"ls","prompt":"x"}`))[0]
	if r.Prompt != "" || r.SubagentType != "" {
		t.Errorf("Bash: prompt %q type %q", r.Prompt, r.SubagentType)
	}
}

func TestReadTaskRecords_SubagentDirectoryOutputPathIsNoPath(t *testing.T) {
	t.Parallel()
	r := readRecords(t,
		namedUseLine("Task", "a1", t0, agentInput),
		plainResultLine("a1", t1, false, "launched", `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`),
	)[0]
	if r.OutputPath != "" || r.ID != "ag1" {
		t.Errorf("rec = %+v", r)
	}
}

func TestReadTaskRecords_LaunchResultWithoutToolUseHasNoCallID(t *testing.T) {
	t.Parallel()
	r := readRecords(t,
		plainResultLine("a1", t1, false, "launched", `{"isAsync":true,"status":"async_launched","agentId":"ag1"}`),
	)[0]
	if r.CallID != "" || r.ID != "ag1" {
		t.Errorf("rec = %+v", r)
	}
}

func twoUseLine(first, second string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":"m-two","content":[%s,%s]}}`, t0, first, second)
}

func useBlock(name, id, input string) string {
	return fmt.Sprintf(`{"type":"tool_use","id":%q,"name":%q,"input":%s}`, id, name, input)
}

func recordsByLabel(recs []TaskRecord) map[string]TaskRecord {
	out := map[string]TaskRecord{}
	for _, r := range recs {
		out[r.Label] = r
	}
	return out
}

func TestReadTaskRecords_StringRunInBackground(t *testing.T) {
	t.Parallel()
	recs := readRecords(t,
		twoUseLine(
			useBlock("Agent", "a1", `{"description":"explore","run_in_background":"true"}`),
			useBlock("Bash", "b1", `{"command":"make","description":"build","run_in_background":"true"}`)),
		plainResultLine("a1", t1, false, "launched", `{"status":"async_launched","agentId":"ag1","description":"explore"}`),
		namedUseLine("Bash", "b2", t1, `{"command":"ls","description":"list","run_in_background":"false"}`),
	)
	got := recordsByLabel(recs)
	if r := got["explore"]; len(recs) != 3 || r.Kind != TaskKindSubagent || r.CallID != "a1" || r.ID != "ag1" {
		t.Errorf("records = %+v, want the Agent call with its label and CallID", recs)
	}
	if got["build"].Kind != TaskKindShell || got["list"].Kind != TaskKindCall {
		t.Errorf("build %q list %q, want shell and call", got["build"].Kind, got["list"].Kind)
	}
}

func TestReadTaskRecords_UndecodableBlockKeepsSiblings(t *testing.T) {
	t.Parallel()
	recs := readRecords(t, twoUseLine(
		useBlock("Agent", "a1", `{"description":5}`),
		useBlock("Bash", "b1", `{"command":"ls","description":"list"}`)))
	if len(recs) != 1 || recs[0].Label != "list" {
		t.Errorf("records = %+v, want only the sibling", recs)
	}
}

func TestReadTaskRecords_LaunchResultWithoutToolUseTakesItsDescription(t *testing.T) {
	t.Parallel()
	r := readRecords(t,
		plainResultLine("a1", t1, false, "launched", `{"status":"async_launched","agentId":"ag1","description":"explore"}`),
	)[0]
	if r.Label != "explore" || r.CallID != "" {
		t.Errorf("rec = %+v, want label explore and no CallID", r)
	}
}

func TestReadTaskRecords_ShellAndMonitorHaveNoCallID(t *testing.T) {
	t.Parallel()
	for _, r := range readRecords(t,
		namedUseLine("Monitor", "m1", t0, `{"description":"watch","command":"tail -f x"}`),
		namedUseLine("Bash", "b1", t0, `{"command":"ls","run_in_background":true}`),
		namedUseLine("Bash", "b2", t0, `{"command":"ls"}`),
	) {
		if r.CallID != "" {
			t.Errorf("%s record has CallID %q", r.Kind, r.CallID)
		}
	}
}

func TestReadTaskRecords_SyncAgentCall(t *testing.T) {
	t.Parallel()
	r := readRecords(t, namedUseLine("Agent", "a1", t0, agentInput))[0]
	if r.Kind != TaskKindSubagent || r.Label != "explore" || r.End != nil || r.ID != "" || r.CallID != "a1" {
		t.Fatalf("open = %+v", r)
	}
	r = readRecords(t, namedUseLine("Agent", "a1", t0, agentInput), turnEndLine(t2))[0]
	if !r.TurnEnd.Equal(mustParseTime(t2)) {
		t.Errorf("turn end = %v", r.TurnEnd)
	}
	r = readRecords(t, namedUseLine("Agent", "a1", t0, agentInput),
		plainResultLine("a1", t1, true, "bad", `{"agentId":"ag9"}`))[0]
	if r.ID != "ag9" || r.End == nil || r.End.Outcome != TaskOutcomeFailure || r.End.Source != TaskEndToolResult {
		t.Errorf("answered = %+v", r)
	}
}
