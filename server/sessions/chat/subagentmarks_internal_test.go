package chat

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
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

func TestRead_MarksSubagentCallsOnTheWire(t *testing.T) {
	t.Parallel()
	agent := `{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}},{"type":"tool_use","id":"t2","name":"Agent","input":{"description":"Survey","subagent_type":"Explore"}}]}}`
	want := []wire.SessionChatSubagentCall{{Index: 1, CallID: "t2", Type: "Explore", Description: "Survey"}}
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
