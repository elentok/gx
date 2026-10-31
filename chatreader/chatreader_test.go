package chatreader_test

import (
	"strings"
	"testing"

	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

func readFixture(t *testing.T, name string) chatreader.Result {
	t.Helper()
	res, ok, err := chatreader.ReadFile(harness.Codex, "testdata/"+name)
	if err != nil || !ok {
		t.Fatalf("ReadFile(%s) = %v, %v", name, ok, err)
	}
	return res
}

func readCodex(t *testing.T, lines ...string) chatreader.Result {
	t.Helper()
	res, err := chatreader.Read(harness.Codex, strings.NewReader(strings.Join(lines, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func summary(msgs []transcript.ChatMessage) string {
	var out []string
	for _, m := range msgs {
		s := m.ID + ":" + m.Role + ":" + m.Kind + ":" + m.Text
		if m.Kind == transcript.ChatKindTools {
			s += strings.Repeat("*", m.ToolCount) + m.LastTool
		}
		out = append(out, s)
	}
	return strings.Join(out, "|")
}

func TestCodex_PlainTurnHidesDeveloperAndInjectedLines(t *testing.T) {
	t.Parallel()
	res := readFixture(t, "plain-turn.jsonl")
	want := "o11:person:prompt:Reply with exactly the word: pong|o14:agent:text:pong"
	if got := summary(res.Messages); got != want {
		t.Errorf("messages = %s, want %s", got, want)
	}
	if res.Unreadable {
		t.Error("Unreadable = true for a readable rollout")
	}
}

func TestCodex_CompactionIsAnEntry(t *testing.T) {
	t.Parallel()
	res := readFixture(t, "compaction.jsonl")
	want := "o11:person:prompt:Reply with exactly the word: pong|o14:agent:text:pong|o20::compacted:"
	if got := summary(res.Messages); got != want {
		t.Errorf("messages = %s, want %s", got, want)
	}
}

func TestCodex_ToolRunFoldsWithCountAndLastTool(t *testing.T) {
	t.Parallel()
	res := readFixture(t, "approval.jsonl")
	var tools []transcript.ChatMessage
	for _, m := range res.Messages {
		if m.Kind == transcript.ChatKindTools {
			tools = append(tools, m)
		}
	}
	if len(tools) != 1 || tools[0].ID != "o15" || tools[0].ToolCount != 1 || tools[0].LastTool != "exec" {
		t.Errorf("tools = %+v, want one o15 run of 1 exec", tools)
	}
	if last := res.Messages[len(res.Messages)-1]; last.ID != "o21" || last.Kind != transcript.ChatKindText {
		t.Errorf("last = %+v, want the o21 reply", last)
	}
}

func TestCodex_SyntheticLines(t *testing.T) {
	t.Parallel()
	res := readCodex(t,
		`{"ordinal":0,"type":"session_meta","payload":{"cli_version":"9.9.9"}}`,
		`{"ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"$gx-design x"}]}}`,
		`{"ordinal":2,"type":"response_item","payload":{"type":"function_call","name":"shell"}}`,
		`{"ordinal":3,"type":"response_item","payload":{"type":"function_call_output","output":"x"}}`,
		`{"ordinal":4,"type":"response_item","payload":{"type":"custom_tool_call","name":"exec"}}`,
		`{"ordinal":5,"type":"brand_new_line","payload":{"type":"message","role":"assistant"}}`,
		`not json`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
	)
	want := "o1:person:command:$gx-design x|o2:agent:tools:**exec|l8:agent:text:done"
	if got := summary(res.Messages); got != want {
		t.Errorf("messages = %s, want %s", got, want)
	}
}

func TestCodex_InjectedPartsAreDroppedFromAMixedMessage(t *testing.T) {
	t.Parallel()
	res := readCodex(t,
		`{"ordinal":0,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>x</environment_context>"},{"type":"input_text","text":"hello"}]}}`)
	if got, want := summary(res.Messages), "o0:person:prompt:hello"; got != want {
		t.Errorf("messages = %s, want %s", got, want)
	}
}

func TestCodex_TurnsWithoutMessagesAreUnreadable(t *testing.T) {
	t.Parallel()
	res := readCodex(t,
		`{"ordinal":0,"type":"session_meta","payload":{"cli_version":"9.9.9"}}`,
		`{"ordinal":1,"type":"event_msg","payload":{"type":"task_complete"}}`)
	if !res.Unreadable || res.CodexVersion != "9.9.9" || len(res.Messages) != 0 {
		t.Errorf("res = %+v, want unreadable with version 9.9.9", res)
	}
	if res := readCodex(t, `{"ordinal":0,"type":"session_meta","payload":{"cli_version":"9.9.9"}}`); res.Unreadable {
		t.Error("a rollout with no turns yet is not unreadable")
	}
}

func TestClaude_ReadsThroughTheDispatcher(t *testing.T) {
	t.Parallel()
	res, err := chatreader.Read(harness.Claude, strings.NewReader(
		`{"type":"assistant","uuid":"a1","message":{"content":[{"type":"text","text":"hi"}]}}`+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(res.Messages), "a1:agent:text:hi"; got != want {
		t.Errorf("messages = %s, want %s", got, want)
	}
}

func TestReadFile_MissingFile(t *testing.T) {
	t.Parallel()
	if _, ok, err := chatreader.ReadFile(harness.Codex, "testdata/nope.jsonl"); ok || err != nil {
		t.Errorf("ok = %v, err = %v, want false, nil", ok, err)
	}
}
