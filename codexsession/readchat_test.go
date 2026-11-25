package codexsession

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestReadChat_MarksSpawnAgentInRootOnly(t *testing.T) {
	rollout := `{"timestamp":"2026-03-06T18:40:00.000Z","type":"session_meta","payload":{"cli_version":"0.99.0"}}` + "\n" +
		`{"timestamp":"2026-03-06T18:40:01.000Z","ordinal":1,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"sp1","arguments":"{\"task_name\":\"review\"}"}}` + "\n"
	root, err := ReadChat(strings.NewReader(rollout), "")
	if err != nil || len(root.Messages) != 1 {
		t.Fatalf("ReadChat = %+v, %v", root, err)
	}
	want := []transcript.SubagentCall{{Index: 0, CallID: "sp1", Description: "review"}}
	if got := root.Messages[0].SubagentCalls; !reflect.DeepEqual(got, want) {
		t.Errorf("root SubagentCalls = %+v, want %+v", got, want)
	}
	child, err := ReadChatFromOrdinal(strings.NewReader(rollout), "", 1)
	if err != nil || len(child.Messages) != 1 {
		t.Fatalf("ReadChatFromOrdinal = %+v, %v", child, err)
	}
	if got := child.Messages[0].SubagentCalls; got != nil {
		t.Errorf("child SubagentCalls = %+v, want none", got)
	}
}

func TestReadChatFromOrdinal_CutsParentHistoryCopy(t *testing.T) {
	msg := func(ord int, role, kind, text string) string {
		return fmt.Sprintf(`{"timestamp":"2026-03-06T18:40:03.291Z","ordinal":%d,"type":"response_item","payload":{"type":"message","role":"%s","content":[{"type":"%s","text":"%s"}]}}`, ord, role, kind, text)
	}
	lines := []string{
		`{"timestamp":"2026-03-06T18:40:00.000Z","type":"session_meta","payload":{"cli_version":"0.99.0"}}`,
		msg(1, "user", "input_text", "parent prompt"),
		msg(2, "assistant", "output_text", "parent answer"),
		`{"timestamp":"2026-03-06T18:40:03.291Z","ordinal":3,"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":\"ls\"}"}}`,
		`{"timestamp":"2026-03-06T18:40:03.291Z","ordinal":4,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"NEW_TASK child header"}]}}`,
		msg(5, "assistant", "output_text", "child answer"),
		msg(6, "assistant", "output_text", "child more"),
	}
	rollout := strings.Join(lines, "\n") + "\n"

	chat, err := ReadChatFromOrdinal(strings.NewReader(rollout), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if chat.Version != "0.99.0" {
		t.Errorf("Version = %q, want 0.99.0", chat.Version)
	}
	var ids []string
	for _, m := range chat.Messages {
		ids = append(ids, m.ID)
	}
	if got := strings.Join(ids, ","); got != "o5,o6" {
		t.Errorf("message ids = %q, want o5,o6", got)
	}

	all, err := ReadChat(strings.NewReader(rollout), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Messages) != 6 {
		t.Errorf("default read gave %d messages, want 6", len(all.Messages))
	}
}

func TestReadChat_SlashAndGxMessagesAreCommands(t *testing.T) {
	user := func(text string) string {
		return `{"timestamp":"2026-03-06T18:40:03.291Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}`
	}
	lines := []string{
		user("/status"),
		user("plan the work"),
		user("$gx-implement x"),
	}
	chat, err := ReadChat(strings.NewReader(strings.Join(lines, "\n")+"\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind string
		text string
	}{
		{transcript.ChatKindCommand, "/status"},
		{transcript.ChatKindPrompt, "plan the work"},
		{transcript.ChatKindCommand, "$gx-implement x"},
	}
	if len(chat.Messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(chat.Messages), len(want))
	}
	for i, w := range want {
		if m := chat.Messages[i]; m.Kind != w.kind || m.Text != w.text {
			t.Errorf("message %d = %q %q, want %q %q", i, m.Kind, m.Text, w.kind, w.text)
		}
	}
}
