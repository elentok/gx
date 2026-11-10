package chat

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
)

const openQuestionRollout = `{"type":"event_msg","payload":{"type":"task_started"}}
{"type":"response_item","payload":{"type":"function_call","name":"request_user_input","call_id":"call_1","arguments":"{\"questions\":[{\"id\":\"pick\",\"header\":\"Color\",\"question\":\"Which color?\",\"options\":[{\"label\":\"Red\",\"description\":\"Choose red.\"},{\"label\":\"Blue\",\"description\":\"Choose blue.\"}]}]}"}}
`

const questionOutputLine = `{"type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"{}"}}
`

func codexCards(t *testing.T, m Meta, rollout string, more ...string) []wire.QuestionCard {
	t.Helper()
	src := newFakeSource()
	src.codexHome = t.TempDir()
	path := filepath.Join(src.codexHome, "sessions", "rollout-"+testConv+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range more {
		rollout += s
	}
	if err := os.WriteFile(path, []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Deps{Source: src, Log: slog.New(slog.DiscardHandler)})
	return c.Cards(t.TempDir(), m, Snapshot{})
}

func plainCodex() Meta {
	return Meta{ID: "s1", Harness: "codex", ConversationID: testConv}
}

func TestCards_PlainCodexListsTheOpenQuestionCall(t *testing.T) {
	t.Parallel()
	got := codexCards(t, plainCodex(), openQuestionRollout)
	if len(got) != 1 || got[0].Source != wire.CardSourceCodex || got[0].ID != "call_1" {
		t.Fatalf("cards = %+v, want the codex-question card call_1", got)
	}
	q := got[0].Questions[0]
	if q.Label != "Color" || q.Title != "Which color?" || len(q.Options) != 2 ||
		q.Options[1].Label != "Blue" || q.Options[1].Description != "Choose blue." {
		t.Errorf("question = %+v", q)
	}
}

func TestCards_ACodexOutputClosesTheQuestion(t *testing.T) {
	t.Parallel()
	if got := codexCards(t, plainCodex(), openQuestionRollout, questionOutputLine); len(got) != 0 {
		t.Fatalf("cards = %+v, want none", got)
	}
}

func TestCards_NonPlainCodexListsNoQuestionCard(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]Meta{
		"flow":   {ID: "s1", Harness: "codex", ConversationID: testConv, Flow: FlowRef{Flow: "design"}},
		"fix":    {ID: "s1", Harness: "codex", ConversationID: testConv, FixOf: "gx:e/01"},
		"ticket": {ID: "s1", Harness: "codex", ConversationID: testConv, Ticket: "gx:e/01"},
	} {
		if got := codexCards(t, m, openQuestionRollout); len(got) != 0 {
			t.Errorf("%s cards = %+v, want none", name, got)
		}
	}
}
