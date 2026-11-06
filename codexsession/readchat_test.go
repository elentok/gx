package codexsession

import (
	"strings"
	"testing"

	"github.com/elentok/gx/transcript"
)

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
