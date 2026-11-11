package promptflows_test

import (
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/wire"
)

func TestSessionPrompt_HuhQuestionsLandOnTheConversationRecord(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	root := f.h.ConversationsDir
	if err := conversations.Save(root, conversations.Record{ID: f.conv, Ticket: "proj:epic/01"}); err != nil {
		t.Fatal(err)
	}
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{
		ID: f.id, Text: "round answers", Huh: []wire.SessionHuh{{Question: "Which db?", Title: "Database"}},
	})
	if err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	rec, err := conversations.Read(root, f.conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Huh) != 1 || rec.Huh[0].Question != "Which db?" || rec.Huh[0].Title != "Database" || rec.Huh[0].At.IsZero() {
		t.Errorf("huh = %+v; want the one question with its title and time", rec.Huh)
	}
	if rec.Huh[0].Session != f.id || rec.Huh[0].Harness == "" {
		t.Errorf("huh = %+v; want the session id %q and a harness", rec.Huh[0], f.id)
	}
}

func TestSessionPrompt_HuhWritesNothingUntilSentAndEachQuestionAddsARecord(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	root := f.h.ConversationsDir
	if err := conversations.Save(root, conversations.Record{ID: f.conv, Ticket: "proj:epic/01"}); err != nil {
		t.Fatal(err)
	}
	huhCount := func() int {
		rec, err := conversations.Read(root, f.conv)
		if err != nil {
			t.Fatal(err)
		}
		return len(rec.Huh)
	}
	// The click alone sends nothing: no prompt call, so no record.
	if n := huhCount(); n != 0 {
		t.Fatalf("huh records before any send = %d, want 0", n)
	}
	for i, q := range []string{"Which db?", "Which queue?"} {
		res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{
			ID: f.id, Text: "round answers", Huh: []wire.SessionHuh{{Question: q, Title: "T"}},
		})
		if err != nil || res.Refused {
			t.Fatalf("prompt %d = %+v, %v", i, res, err)
		}
		if n := huhCount(); n != i+1 {
			t.Fatalf("huh records after send %d = %d, want %d", i+1, n, i+1)
		}
	}
}
