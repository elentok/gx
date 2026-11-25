package chat

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/server/wire"
)

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
