package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// subagentFixture is a Claude session with one subagent "ag1"; reads counts
// the subagent-file reads.
type subagentFixture struct {
	*readFixture
	file  string
	reads *atomic.Int32
}

func newSubagentFixture(t *testing.T, launch, notification string) *subagentFixture {
	t.Helper()
	config := t.TempDir()
	src := newFakeSource()
	src.meta = Meta{ID: "s1", Harness: "claude", Dir: testCwd, ConversationID: testConv, Flow: FlowRef{Flow: tickets.FlowDesign, Key: "gx:epic/03"}}
	c := claudeChat(src, config)
	reads := &atomic.Int32{}
	c.reads.subagent = func(p string) (transcript.SubagentDetails, error) {
		reads.Add(1)
		return transcript.ReadSubagentFile(p)
	}
	parent := filepath.Join(config, "projects", transcript.Slugify(testCwd), testConv+".jsonl")
	f := &subagentFixture{
		readFixture: &readFixture{src: src, config: config, srv: serveChat(t, src, c), path: parent},
		file:        strings.TrimSuffix(parent, ".jsonl") + "/subagents/agent-ag1.jsonl",
		reads:       reads,
	}
	ts := func(ago time.Duration) string { return time.Now().UTC().Add(-ago).Format(time.RFC3339Nano) }
	lines := []string{
		fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"description":"explore","prompt":"p"}}]}}`, ts(time.Minute)),
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"a1","content":"ok"}]},"toolUseResult":%s}`, ts(50*time.Second), launch),
	}
	if notification != "" {
		lines = append(lines, fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts(10*time.Second), notification))
	}
	// A running call keeps the group open once the subagent has finished.
	lines = append(lines, fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"u9","name":"Bash","input":{"command":"sleep 30","description":"wait"}}]}}`, ts(55*time.Second)))
	writeLines(t, parent, lines...)
	return f
}

func agentNotification(status, usage string) string {
	return "<task-notification>\n<task-id>ag1</task-id>\n<status>" + status + "</status>\n<summary>s</summary>\n" + usage + "</task-notification>"
}

const subagentFileBody = `{"isSidechain":true,"type":"assistant","message":{"id":"m1","model":"claude-x","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls /private/tmp"}}]}}`

func (f *subagentFixture) agent(t *testing.T) wire.SessionTask {
	t.Helper()
	for _, row := range f.chat(t, "/v1/sessions/chat").Tasks {
		if row.Kind == transcript.TaskKindSubagent {
			return row
		}
	}
	t.Fatal("no subagent row")
	return wire.SessionTask{}
}

func TestRead_SubagentDetailsFromOutputFileAndGrowthRefreshes(t *testing.T) {
	t.Parallel()
	link := filepath.Join(t.TempDir(), "tasks", "ag1.output")
	f := newSubagentFixture(t, fmt.Sprintf(`{"status":"async_launched","agentId":"ag1","resolvedModel":"m","outputFile":%q}`, link), "")
	target := filepath.Join(t.TempDir(), "elsewhere.jsonl")
	writeLines(t, target, subagentFileBody)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got := f.agent(t)
	if got.Model != "claude-x" || got.Tokens != 150 || got.ToolCalls != 1 || got.CurrentCall != "ls /private/tmp" {
		t.Fatalf("row = %+v", got)
	}
	writeLines(t, target, subagentFileBody,
		`{"isSidechain":true,"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}`,
		`{"isSidechain":true,"type":"assistant","message":{"id":"m2","model":"claude-x","usage":{"input_tokens":10,"output_tokens":5},"content":[{"type":"text","text":"hi"}]}}`)
	if got = f.agent(t); got.Tokens != 165 || got.CurrentCall != "" {
		t.Fatalf("after growth = %+v (parent file unchanged)", got)
	}
}

func TestRead_SubagentRowCarriesCallIDAndOtherRowsDoNot(t *testing.T) {
	t.Parallel()
	f := newSubagentFixture(t, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`, "")
	rows := f.chat(t, "/v1/sessions/chat").Tasks
	subs := 0
	for _, row := range rows {
		switch {
		case row.Kind == transcript.TaskKindSubagent && row.CallID == "a1":
			subs++
		case row.CallID != "":
			t.Errorf("%s row has call id %q", row.Kind, row.CallID)
		}
	}
	if subs != 1 {
		t.Fatalf("subagent rows with call id a1 = %d, rows = %+v", subs, rows)
	}
}

func TestRead_SubagentFallsBackToFileNextToTheParent(t *testing.T) {
	t.Parallel()
	f := newSubagentFixture(t, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`, "")
	writeLines(t, f.file, subagentFileBody)
	if got := f.agent(t); got.Tokens != 150 || got.ToolCalls != 1 {
		t.Fatalf("row = %+v", got)
	}
}

func TestRead_FinishedSubagentWithUsageNeverOpensItsFile(t *testing.T) {
	t.Parallel()
	usage := "<usage><subagent_tokens>900</subagent_tokens><tool_uses>7</tool_uses></usage>\n"
	f := newSubagentFixture(t, `{"status":"async_launched","agentId":"ag1","resolvedModel":"launch-model","outputFile":"/x/tasks/"}`, agentNotification("completed", usage))
	writeLines(t, f.file, subagentFileBody)
	got := f.agent(t)
	if got.Tokens != 900 || got.ToolCalls != 7 || got.Model != "launch-model" || got.CurrentCall != "" || f.reads.Load() != 0 {
		t.Fatalf("row = %+v, file reads = %d", got, f.reads.Load())
	}
}

func TestRead_FinishedSubagentWithoutUsageReadsItsFileOnce(t *testing.T) {
	t.Parallel()
	f := newSubagentFixture(t, `{"status":"async_launched","agentId":"ag1","outputFile":"/x/tasks/"}`, agentNotification("failed", ""))
	writeLines(t, f.file, subagentFileBody)
	for range 2 {
		if got := f.agent(t); got.Tokens != 150 || got.CurrentCall != "" {
			t.Fatalf("row = %+v", got)
		}
	}
	if f.reads.Load() != 1 {
		t.Fatalf("parsed %d times", f.reads.Load())
	}
}

func TestRead_SubagentWithNoAgentIDGetsNoDetails(t *testing.T) {
	t.Parallel()
	f := newSubagentFixture(t, `{"status":"completed"}`, "")
	if got := f.agent(t); got.Tokens != 0 || f.reads.Load() != 0 {
		t.Fatalf("row = %+v, reads = %d", got, f.reads.Load())
	}
}
