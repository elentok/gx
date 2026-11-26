package chat

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

func TestRead_StringRunInBackgroundKeepsTaskRowsNamed(t *testing.T) {
	t.Parallel()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	use := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"t1","name":"Agent","input":{"description":"Survey","run_in_background":"true"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"make","description":"build","run_in_background":"true"}}]}}`, ts)
	result := func(id, res string) string {
		return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]},"toolUseResult":%s}`, ts, id, res)
	}
	got := chatOf(t, use,
		result("t1", `{"status":"async_launched","agentId":"ag1","description":"Survey"}`),
		result("t2", `{"backgroundTaskId":"b1"}`),
	).Tasks

	rows := map[string]wire.SessionTask{}
	for _, row := range got {
		rows[row.Label] = row
	}
	if len(got) != 2 || rows["Survey"].CallID != "t1" || rows["build"].Kind != "shell" {
		t.Errorf("tasks = %+v, want Survey with call_id t1 and the build shell", got)
	}
}

// callStates maps each subagent call of a read to its state.
func callStates(res wire.SessionChatResponse) map[string]string {
	states := map[string]string{}
	for _, m := range res.Messages {
		for _, call := range m.SubagentCalls {
			states[call.CallID] = call.State
		}
	}
	return states
}

func TestRead_ClaudeSubagentCallsCarryTheirState(t *testing.T) {
	t.Parallel()
	use := func(id, ts string) string {
		return fmt.Sprintf(`{"type":"assistant",%s"message":{"content":[{"type":"tool_use","id":%q,"name":"Agent","input":{"description":%q}}]}}`, ts, id, id)
	}
	at := func(ago time.Duration) string { return fmt.Sprintf(`"timestamp":%q,`, pageTS(ago)) }
	done := func(id string, ago time.Duration) string {
		return fmt.Sprintf(`{"type":"user",%s"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]},"toolUseResult":{"status":"completed"}}`, at(ago), id)
	}
	got := chatOf(t,
		use("old", at(10*time.Minute)), done("old", 9*time.Minute),
		use("run", at(2*time.Minute)),
		use("done", at(time.Minute)), done("done", 30*time.Second),
		// The task scan skips a line with no timestamp.
		use("unscanned", ""),
	)

	want := map[string]string{
		"old": wire.SubagentCallFinished, "done": wire.SubagentCallFinished,
		"run": wire.SubagentCallRunning, "unscanned": wire.SubagentCallNoRecord,
	}
	if states := callStates(got); !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	rows := map[string]bool{}
	for _, row := range got.Tasks {
		rows[row.CallID] = row.Finished
	}
	if want := map[string]bool{"run": false, "done": true}; !reflect.DeepEqual(rows, want) {
		t.Errorf("task rows finished = %v, want %v: the older group is closed", rows, want)
	}
}

func TestRead_SubagentCallStateIgnoresTheSidecarMarker(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", `{"toolUseId":"a1","agentType":"Explore","finished":true}`, pagePrompt)
	got := callStates(f.chat(t, "/v1/sessions/chat"))
	if got["a1"] != wire.SubagentCallRunning {
		t.Errorf("states = %v, want a1 running: its record has not ended", got)
	}
}

func TestRead_SubagentCallStateIsDecidedOnEveryRead(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	c := claudeChat(f.src, f.config)
	start := time.Now()
	now := start
	c.now = func() time.Time { return now }
	f.srv = serveChat(t, f.src, c)
	writeLines(t, f.path, pageAgentCall(time.Minute))
	read := func(step, want string) {
		t.Helper()
		if got := callStates(f.chat(t, "/v1/sessions/chat")); got["a1"] != want {
			t.Errorf("%s: states = %v, want a1 %s", step, got, want)
		}
	}

	read("first read", wire.SubagentCallRunning)
	now = start.Add(transcript.BackgroundTaskAgedOutCap)
	read("aged out", wire.SubagentCallFinished)
	now = start
	read("clock back", wire.SubagentCallRunning)
	f.src.meta.Status.State = wire.SessionExited
	read("session exited", wire.SubagentCallFinished)
}

func TestRead_MarksSubagentCallsOnTheWire(t *testing.T) {
	t.Parallel()
	agent := `{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}},{"type":"tool_use","id":"t2","name":"Agent","input":{"description":"Survey","subagent_type":"Explore"}}]}}`
	want := []wire.SessionChatSubagentCall{{Index: 1, CallID: "t2", Type: "Explore", Description: "Survey", State: wire.SubagentCallFinished}}
	got := chatOf(t, agent)
	if len(got.Messages) != 1 || !reflect.DeepEqual(got.Messages[0].SubagentCalls, want) {
		t.Errorf("parent read = %+v, want mark %+v", got.Messages, want)
	}

	f := newPageFixture(t, false, agent)
	nested := `{"type":"assistant","isSidechain":true,"uuid":"n1","timestamp":"2026-01-01T10:00:05Z","message":{"content":[{"type":"tool_use","id":"n2","name":"Agent","input":{"description":"Deeper"}}]}}`
	f.subagent(t, "x", pageMeta, pagePrompt, nested)
	var marks []wire.SessionChatSubagentCall
	for _, m := range f.page(t).Messages {
		marks = append(marks, m.SubagentCalls...)
	}
	if wantNested := []wire.SessionChatSubagentCall{{Index: 0, CallID: "n2", Description: "Deeper"}}; !reflect.DeepEqual(marks, wantNested) {
		t.Errorf("subagent read marks = %+v, want %+v", marks, wantNested)
	}
}
