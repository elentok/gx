package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
)

func nestedAgentCall(id, title string) string {
	return fmt.Sprintf(`{"isSidechain":true,"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":"Agent","input":{"description":%q,"prompt":"dig"}}]}}`, pageTS(30*time.Second), id, title)
}

// nestedFixture is a chain: the parent's call a1 started agent x, whose call n1
// started agent y, whose call m1 started agent z.
func nestedFixture(t *testing.T) *pageFixture {
	t.Helper()
	f := newPageFixture(t, true, pageAgentCall(time.Minute))
	f.subagent(t, "x", pageMeta, pagePrompt, nestedAgentCall("n1", "inner"))
	f.subagent(t, "y", `{"toolUseId":"n1","agentType":"Plan","parentAgentId":"x"}`, pagePrompt, nestedAgentCall("m1", "innermost"))
	f.subagent(t, "z", `{"toolUseId":"m1","agentType":"general","parentAgentId":"y"}`, pagePrompt, subagentFileBody)
	return f
}

func (f *pageFixture) pageOf(t *testing.T, call string) *subagent.SessionSubagent {
	t.Helper()
	code, body := f.getQuery(t, "/v1/sessions/chat", "subagent="+call)
	got, err := decodeChat(body)
	if err != nil || code != 200 || got.Subagent == nil {
		t.Fatalf("page %s = %d %q (%v)", call, code, body, err)
	}
	return got.Subagent
}

func decodeChat(body []byte) (wire.SessionChatResponse, error) {
	var got wire.SessionChatResponse
	err := json.Unmarshal(body, &got)
	return got, err
}

func TestSubagentPage_NestedCallNamesItsParentAndChain(t *testing.T) {
	t.Parallel()
	f := nestedFixture(t)
	h := f.pageOf(t, "m1")
	want := []subagent.SessionSubagentLink{{CallID: "a1", Title: "explore"}, {CallID: "n1", Title: "inner"}}
	if h.ParentCallID != "n1" || !reflect.DeepEqual(h.Chain, want) {
		t.Fatalf("parent %q chain %+v, want n1 %+v", h.ParentCallID, h.Chain, want)
	}
	if h.Title != "innermost" || h.Type != "general" || h.RootSessionID != "s1" || h.CallID != "m1" {
		t.Fatalf("header = %+v", h)
	}
	top := f.pageOf(t, "n1")
	if top.ParentCallID != "a1" || len(top.Chain) != 1 || top.Chain[0].Title != "explore" || top.Title != "inner" {
		t.Fatalf("second level = %+v", top)
	}
	if first := f.pageOf(t, "a1"); first.ParentCallID != "" || len(first.Chain) != 0 {
		t.Fatalf("top level has a chain: %+v", first)
	}
}

func TestSubagentPage_NestedCallOfAnExitedSessionIsStopped(t *testing.T) {
	t.Parallel()
	f := nestedFixture(t)
	f.src.meta.Status.State = wire.SessionExited
	if h := f.pageOf(t, "m1"); h.Status != subagent.SubagentStatusStopped {
		t.Fatalf("status = %q", h.Status)
	}
}
