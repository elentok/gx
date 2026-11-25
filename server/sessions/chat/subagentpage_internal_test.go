package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
	"github.com/elentok/gx/transcript"
)

func pageTS(ago time.Duration) string {
	return time.Now().UTC().Add(-ago).Format(time.RFC3339Nano)
}

func pageAgentCall(ago time.Duration) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"description":"explore","prompt":"find foo"}}]}}`, pageTS(ago))
}

func pageResult(ago time.Duration, toolUseResult string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"a1","content":"ok"}]},"toolUseResult":%s}`, pageTS(ago), toolUseResult)
}

func pageNotice(ago time.Duration, status string) string {
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, pageTS(ago), agentNotification(status, ""))
}

func pageBash(ago time.Duration) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"u9","name":"Bash","input":{"command":"sleep 30","description":"wait"}}]}}`, pageTS(ago))
}

const (
	pagePrompt  = `{"isSidechain":true,"type":"user","uuid":"p1","message":{"role":"user","content":"find foo"}}`
	pageBashRes = `{"isSidechain":true,"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}`
	pageHandbk  = `{"isSidechain":true,"type":"assistant","uuid":"h1","message":{"id":"m3","model":"claude-x","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"tool_use","id":"hb","name":"SubagentHandback","input":{"message":"it is in foo.go"}}]}}`
	pageFinal   = `{"isSidechain":true,"type":"assistant","uuid":"f1","message":{"id":"m4","model":"claude-x","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"text","text":"all done"}]}}`
)

// pageFixture is a Claude session whose parent holds call a1.
type pageFixture struct {
	*readFixture
	dir string
	// pageReads counts the page's reads of a subagent file.
	pageReads *atomic.Int32
}

// newPageFixture writes the parent transcript; bash adds a running Bash call,
// which keeps the task group open.
func newPageFixture(t *testing.T, bash bool, parent ...string) *pageFixture {
	t.Helper()
	f := newClaudeFixture(t)
	c := claudeChat(f.src, f.config)
	reads := &atomic.Int32{}
	c.reads.subagentPage = func(string) { reads.Add(1) }
	f.srv = serveChat(t, f.src, c)
	if bash {
		parent = append(parent, pageBash(55*time.Second))
	}
	writeLines(t, f.path, parent...)
	return &pageFixture{readFixture: f, dir: strings.TrimSuffix(f.path, ".jsonl") + "/subagents", pageReads: reads}
}

// subagent writes the subagent file and its sidecar for agent id.
func (f *pageFixture) subagent(t *testing.T, id, meta string, lines ...string) {
	t.Helper()
	writeLines(t, filepath.Join(f.dir, "agent-"+id+".jsonl"), lines...)
	if meta != "" {
		writeLines(t, filepath.Join(f.dir, "agent-"+id+".meta.json"), meta)
	}
}

func (f *pageFixture) page(t *testing.T) wire.SessionChatResponse {
	t.Helper()
	code, body := f.getQuery(t, "/v1/sessions/chat", "subagent=a1")
	var got wire.SessionChatResponse
	if err := json.Unmarshal(body, &got); err != nil || code != http.StatusOK || got.Subagent == nil {
		t.Fatalf("page = %d %q (%v)", code, body, err)
	}
	return got
}

const pageMeta = `{"toolUseId":"a1","agentType":"Explore"}`

func TestSubagentPage_Status(t *testing.T) {
	t.Parallel()
	const long = 20 * time.Minute
	cases := []struct {
		name    string
		parent  []string
		id      string
		meta    string
		file    []string
		exited  bool
		status  string
		result  string
		current bool
	}{
		{"running foreground", []string{pageAgentCall(time.Minute)}, "x", pageMeta,
			[]string{pagePrompt, subagentFileBody}, false, subagent.SubagentStatusRunning, "", true},
		{"returned with handback", []string{pageAgentCall(time.Minute), pageResult(10*time.Second, `{"status":"completed"}`)}, "x", pageMeta,
			[]string{pagePrompt, subagentFileBody, pageBashRes, pageHandbk}, false, subagent.SubagentStatusDone, "it is in foo.go", false},
		{"returned without handback", []string{pageAgentCall(time.Minute), pageResult(10*time.Second, `{"status":"completed"}`)}, "x", pageMeta,
			[]string{pagePrompt, subagentFileBody, pageBashRes, pageFinal}, false, subagent.SubagentStatusStopped, "", false},
		{"finished marker without handback", []string{pageAgentCall(time.Minute)}, "x", `{"toolUseId":"a1","agentType":"Explore","finished":true}`,
			[]string{pagePrompt, subagentFileBody, pageBashRes, pageFinal}, false, subagent.SubagentStatusStopped, "all done", false},
		{"background returned long ago", []string{pageAgentCall(long), pageResult(long-time.Second, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`)}, "ag1", pageMeta,
			[]string{pagePrompt, subagentFileBody}, false, subagent.SubagentStatusRunning, "", true},
		{"background ended with handback", []string{pageAgentCall(long), pageResult(long-time.Second, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`), pageNotice(time.Minute, "completed")}, "ag1", pageMeta,
			[]string{pagePrompt, subagentFileBody, pageBashRes, pageHandbk}, false, subagent.SubagentStatusDone, "it is in foo.go", false},
		{"background ended without handback", []string{pageAgentCall(long), pageResult(long-time.Second, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`), pageNotice(time.Minute, "failed")}, "ag1", pageMeta,
			[]string{pagePrompt, subagentFileBody, pageBashRes, pageFinal}, false, subagent.SubagentStatusStopped, "", false},
		{"exited session", []string{pageAgentCall(time.Minute)}, "x", pageMeta,
			[]string{pagePrompt, subagentFileBody}, true, subagent.SubagentStatusStopped, "", false},
	}
	for _, tc := range cases {
		for _, groupOpen := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/group open %v", tc.name, groupOpen), func(t *testing.T) {
				t.Parallel()
				f := newPageFixture(t, groupOpen, tc.parent...)
				f.subagent(t, tc.id, tc.meta, tc.file...)
				if tc.exited {
					f.src.meta.Status.State = wire.SessionExited
				}
				got := f.page(t)
				h := got.Subagent
				if h.Status != tc.status || h.Result != tc.result || (got.Current != nil) != tc.current {
					t.Fatalf("status %q result %q current %+v; want %q %q current %v", h.Status, h.Result, got.Current, tc.status, tc.result, tc.current)
				}
				if h.CallID != "a1" || h.RootSessionID != "s1" || h.Title != "explore" || h.Type != "Explore" || h.Prompt != "find foo" {
					t.Fatalf("header = %+v", h)
				}
				for _, m := range got.Messages {
					if m.Role == transcript.ChatRolePerson {
						t.Errorf("the prompt is repeated in Messages: %+v", m)
					}
					if strings.Contains(strings.Join(m.Calls, "|"), transcript.SubagentHandbackTool) {
						t.Errorf("the handback call is in Calls: %v", m.Calls)
					}
				}
			})
		}
	}
}

func TestSubagentPage_HeaderEqualsTheTaskRow(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", pageMeta, pagePrompt, subagentFileBody)
	var row wire.SessionTask
	for _, r := range f.chat(t, "/v1/sessions/chat").Tasks {
		if r.Kind == transcript.TaskKindSubagent {
			row = r
		}
	}
	h := f.page(t).Subagent
	if row.Tokens != 150 || h.Model != row.Model || h.Tokens != row.Tokens || h.ToolCalls != row.ToolCalls || h.StartedAt != row.StartedAt || h.EndedAt != "" {
		t.Fatalf("header %+v, row %+v", h, row)
	}
}

func TestSubagentPage_UnknownCallIs404(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", pageMeta, pagePrompt, subagentFileBody)
	code, body := f.getQuery(t, "/v1/sessions/chat", "subagent=nope")
	if code != http.StatusNotFound || strings.TrimSpace(string(body)) != "no such subagent" {
		t.Fatalf("= %d %q", code, body)
	}
}

func TestSubagentPage_UnknownSessionKeepsItsOwnMessage(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.src.res = refuse(wire.ReasonSessionUnknown, "no such session")
	code, body := f.getQuery(t, "/v1/sessions/chat", "subagent=a1")
	if code != http.StatusNotFound || strings.TrimSpace(string(body)) != "no such session" {
		t.Fatalf("= %d %q", code, body)
	}
}

func TestSubagentPage_AbsentFieldsAreOmittedAndNoDurationHasNoEnd(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", `{"toolUseId":"a1"}`, pagePrompt, subagentFileBody)
	f.src.meta.Status.State = wire.SessionExited
	_, body := f.getQuery(t, "/v1/sessions/chat", "subagent=a1")
	var raw struct {
		Subagent map[string]any `json:"subagent"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"type", "ended_at", "result", "chain", "unavailable", "parent_call_id"} {
		if _, ok := raw.Subagent[key]; ok {
			t.Errorf("%s is present: %s", key, body)
		}
	}
	if raw.Subagent["status"] != subagent.SubagentStatusStopped {
		t.Errorf("status = %v", raw.Subagent["status"])
	}
}

func TestSubagentPage_WithoutSubagentThereIsNoHeader(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	if got := f.chat(t, "/v1/sessions/chat"); got.Subagent != nil {
		t.Fatalf("header = %+v", got.Subagent)
	}
}

func TestSubagentPage_CodexSessionDoesNotPanic(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path, `{"timestamp":"2026-01-01T10:00:00Z","ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`)
	code, _ := f.getQuery(t, "/v1/sessions/chat", "subagent=a1")
	if code != http.StatusNotFound {
		t.Fatalf("code = %d", code)
	}
}

func TestSubagentPage_CostParsesAnUnchangedFileOnce(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", pageMeta, pagePrompt, subagentFileBody)
	f.page(t)
	f.page(t)
	if f.pageReads.Load() != 1 {
		t.Fatalf("parsed %d times", f.pageReads.Load())
	}
	f.subagent(t, "x", pageMeta, pagePrompt, subagentFileBody, pageBashRes)
	f.page(t)
	if f.pageReads.Load() != 2 {
		t.Fatalf("after growth parsed %d times", f.pageReads.Load())
	}
}

func TestSubagentPage_ChatReadWithoutSubagentOpensNoPageFile(t *testing.T) {
	t.Parallel()
	f := newPageFixture(t, false, pageAgentCall(time.Minute))
	f.subagent(t, "x", pageMeta, pagePrompt, subagentFileBody)
	f.chat(t, "/v1/sessions/chat")
	if f.pageReads.Load() != 0 {
		t.Fatalf("opened %d subagent files", f.pageReads.Load())
	}
}
