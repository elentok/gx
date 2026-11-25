package questionseed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets/ticketlog"
)

func TestSeed_EarlierRoundsAreAnsweredAndTheLastQuestionIsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-x.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\nstatus: paused\n---\n\n# X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Seed(path, Question{
		Summary: "open?", Handoff: "notes",
		Earlier: []Round{
			{Summary: "first?", Handoff: "h", Reply: "yes"},
			{Summary: "second?", Handoff: "h", Choice: "a", Note: "because"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs, err := ticketlog.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	rounds := ticketlog.AllRounds(evs)
	if len(rounds) != 3 {
		t.Fatalf("got %d rounds, want 3", len(rounds))
	}
	if open := ticketlog.OpenQuestion(evs); open == nil || open.Summary != "open?" || open.By != "agent" {
		t.Errorf("open question = %+v, want the agent's open?", open)
	}
	var notes []string
	for _, ev := range evs {
		if ev.Type == ticketlog.EventPauseAnswer {
			notes = append(notes, ev.Note)
		}
	}
	if len(notes) != 2 || notes[1] != "because" {
		t.Errorf("answer notes = %q, want the second to be because", notes)
	}
}
