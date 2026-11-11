package promptflows_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/wire"
)

const answerRoundText = "❓ **Q1** - **Which store?**: SQLite or files?\n\n```gx-question\ntype: options\noptions:\n  - id: a\n    label: SQLite\n  - id: b\n    label: Files\n```\n\n---\n\n" +
	"❓ **Q2** - **Keep going?**: Ok?\n\n```gx-question\ntype: yes-no\nyes: ship it\n```\n\n---\n\n" +
	"❓ **Q3** - **Name?**: What name?"

// printRound writes a transcript whose last assistant message is a printed
// round, and returns its card's id.
func (f promptFixture) printRound(t *testing.T) string {
	t.Helper()
	text, _ := json.Marshal(answerRoundText)
	line := `{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":` + string(text) + `}]}}` + "\n"
	if err := os.MkdirAll(filepath.Dir(f.transcript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.transcript, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Questions) != 1 || chat.Questions[0].Source != wire.CardSourceRound {
		t.Fatalf("questions = %+v, want one round card", chat.Questions)
	}
	return chat.Questions[0].ID
}

func (f promptFixture) answer(t *testing.T, req wire.SessionAnswerRequest) wire.SessionResult {
	t.Helper()
	req.ID = f.id
	res, err := f.h.Client.Sessions.Answer(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionAnswer_RoundPromptFormat(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id := f.printRound(t)
	log := ""
	for _, tc := range []struct {
		name string
		req  wire.SessionAnswerRequest
		want string
	}{
		{"options and yes-no and text",
			wire.SessionAnswerRequest{Answers: []wire.SessionAnswer{{Choice: "b"}, {Choice: "yes"}, {Text: "  Rex "}}},
			"Q1 - Which store?: Files\nQ2 - Keep going?: Yes - ship it\nQ3 - Name?: Rex",
		},
		{"unanswered and a no without a consequence",
			wire.SessionAnswerRequest{Answers: []wire.SessionAnswer{{}, {Choice: "no"}}},
			"Q1 - Which store?: (unanswered)\nQ2 - Keep going?: No\nQ3 - Name?: (unanswered)",
		},
		{"huh with notes",
			wire.SessionAnswerRequest{Answers: []wire.SessionAnswer{{Huh: true, Choice: "a", Note: " why? "}, {Choice: "yes"}, {Text: "x"}}, Note: " overall "},
			"Q1 - Which store?: I could not follow this question. Could you explain it another way?\n  Note: why?\nQ2 - Keep going?: Yes - ship it\nQ3 - Name?: x\n\nNote:\noverall",
		},
	} {
		tc.req.Source, tc.req.CardID = wire.CardSourceRound, id
		if res := f.answer(t, tc.req); res.Refused {
			t.Fatalf("%s: answer = %+v", tc.name, res)
		}
		log += strconv.Quote(tc.want) + "\n"
		waitForSubmitted(t, f.conv, log)
	}
}

func TestSessionAnswer_IsByteIdenticalToThePromptRoute(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id := f.printRound(t)
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourceRound, CardID: id, Answers: []wire.SessionAnswer{{Choice: "a"}, {Choice: "yes"}, {Text: "Rex"}}, Note: "n"}); res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	by := strconv.Quote("Q1 - Which store?: SQLite\nQ2 - Keep going?: Yes - ship it\nQ3 - Name?: Rex\n\nNote:\nn") + "\n"
	waitForSubmitted(t, f.conv, by)
	g := startPromptSession(t)
	if res := g.prompt(t, "Q1 - Which store?: SQLite\nQ2 - Keep going?: Yes - ship it\nQ3 - Name?: Rex\n\nNote:\nn"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, g.conv, by)
}

func TestSessionAnswer_ImagesAndHuhRideWithThePrompt(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id := f.printRound(t)
	root := f.h.ConversationsDir
	if err := conversations.Save(root, conversations.Record{ID: f.conv, Ticket: "proj:epic/01"}); err != nil {
		t.Fatal(err)
	}
	ids, paths := f.upload(t, 1)
	res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourceRound, CardID: id, Images: ids, Answers: []wire.SessionAnswer{{Huh: true}}})
	if res.Refused {
		t.Fatalf("answer = %+v", res)
	}
	want := "Q1 - Which store?: I could not follow this question. Could you explain it another way?\nQ2 - Keep going?: (unanswered)\nQ3 - Name?: (unanswered)"
	waitForSubmitted(t, f.conv, wantEntry(want, paths...))
	rec, err := conversations.Read(root, f.conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Huh) != 1 || rec.Huh[0].Title != "Which store?" {
		t.Errorf("huh = %+v, want the Huh? question", rec.Huh)
	}
}

func TestSessionAnswer_StaleAndUnsupportedSendNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id := f.printRound(t)
	answers := []wire.SessionAnswer{{Choice: "a"}}
	if res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourceRound, CardID: "old", Answers: answers}); !res.Refused || res.Reason != wire.ReasonStale {
		t.Errorf("stale id: %+v, want %s", res, wire.ReasonStale)
	}
	if res := f.answer(t, wire.SessionAnswerRequest{Source: "bogus", CardID: id, Answers: answers}); !res.Refused || res.Reason != wire.ReasonAnswerSourceNotSupported {
		t.Errorf("other source: %+v, want %s", res, wire.ReasonAnswerSourceNotSupported)
	}
	f.fixture().NothingSent(t)
}
