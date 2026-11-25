package transcript

import (
	"slices"
	"strings"
	"testing"
)

// side marks a fixture line as a sidechain line, as every line of a subagent
// file is.
func side(line string) string {
	return strings.Replace(line, `{"type":`, `{"isSidechain":true,"type":`, 1)
}

func sideLines(lines ...string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = side(l)
	}
	return out
}

func subagentFixture() []string {
	return sideLines(
		`{"type":"user","uuid":"p","timestamp":"2026-01-01T10:00:00Z","message":{"content":"find the bug"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"Looking"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-01-01T10:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"out"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-01-01T10:00:04Z","message":{"content":[{"type":"tool_use","id":"h1","name":"SubagentHandback","input":{"message":"it is in foo.go"}}]}}`,
	)
}

func TestReadSubagentChatFrom_KeepsSidechainLines(t *testing.T) {
	t.Parallel()
	body := strings.Join(subagentFixture(), "\n") + "\n"
	msgs, result, err := ReadSubagentChatFrom(strings.NewReader(body), "")
	if err != nil {
		t.Fatal(err)
	}
	type view struct{ Role, Kind, Text string }
	var got []view
	for _, m := range msgs {
		got = append(got, view{m.Role, m.Kind, m.Text})
	}
	want := []view{
		{ChatRolePerson, ChatKindPrompt, "find the bug"},
		{ChatRoleAgent, ChatKindText, "Looking"},
		{ChatRoleAgent, ChatKindTools, ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("messages = %+v, want %+v", got, want)
	}
	tools := msgs[2]
	if tools.ToolCount != 1 || !slices.Equal(tools.Calls, []string{"Bash: ls"}) {
		t.Errorf("tools = %d %v, want one Bash call and no handback", tools.ToolCount, tools.Calls)
	}
	if result != "it is in foo.go" {
		t.Errorf("result = %q", result)
	}
}

func TestReadSubagentChatFrom_NoHandbackHasNoResult(t *testing.T) {
	t.Parallel()
	body := strings.Join(subagentFixture()[:4], "\n") + "\n"
	_, result, err := ReadSubagentChatFrom(strings.NewReader(body), "")
	if err != nil || result != "" {
		t.Errorf("result = %q, err = %v, want empty", result, err)
	}
}

func TestReadChatFrom_DropsSubagentFileLines(t *testing.T) {
	t.Parallel()
	body := strings.Join(subagentFixture(), "\n") + "\n"
	msgs, err := ReadChatFrom(strings.NewReader(body), "")
	if err != nil || len(msgs) != 0 {
		t.Errorf("msgs = %+v, err = %v, want none", msgs, err)
	}
}

func TestReadSubagentTaskRecords_NestedAgentCall(t *testing.T) {
	t.Parallel()
	path := writeTranscript(t, sideLines(
		namedUseLine("Agent", "n1", t0, agentInput),
		plainResultLine("n1", t1, false, "done", `{"status":"completed"}`),
	)...)
	recs, _, err := ReadSubagentTaskRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Kind != TaskKindSubagent || recs[0].Label != "explore" || recs[0].CallID != "n1" {
		t.Fatalf("records = %+v, want one subagent n1", recs)
	}
	off, _, err := ReadTaskRecords(path)
	if err != nil || len(off) != 0 {
		t.Errorf("normal read = %+v, err = %v, want none", off, err)
	}
}

func TestReadSubagentInFlightTools_OpenSidechainCall(t *testing.T) {
	t.Parallel()
	path := writeTranscript(t,
		ifUse("a", "Bash", `{"command":"sleep 1"}`, true),
		ifUse("b", "Bash", `{"command":"make"}`, true),
		ifResult("a"),
	)
	got, err := ReadSubagentInFlightTools(path)
	if err != nil || len(got) != 1 || got[0].Label != "make" {
		t.Errorf("in flight = %+v, err = %v, want only make", got, err)
	}
	off, err := ReadInFlightTools(path)
	if err != nil || len(off) != 0 {
		t.Errorf("normal read = %+v, err = %v, want none", off, err)
	}
}

func TestReadSubagentTaskRecords_OpenCallsMatchReadSubagentInFlightTools(t *testing.T) {
	t.Parallel()
	path := writeTranscript(t, sideLines(
		`{"type":"user","message":{"content":"go"}}`,
		bashUseLine("a", t0, `{"command":"sleep 1"}`),
		bashUseLine("b", t0, `{"command":"make","run_in_background":true}`),
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"g","name":"Grep","input":{"pattern":"foo"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"a","content":"ok"}]}}`,
	)...)
	want, err := ReadSubagentInFlightTools(path)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := ReadSubagentTaskRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 2 || !slices.Equal(got, want) {
		t.Errorf("open calls = %+v, want %+v (two calls)", got, want)
	}
}
