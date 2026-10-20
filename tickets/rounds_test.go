package tickets

import (
	"strings"
	"testing"
)

func TestRenderRounds_NoRoundsIsEmpty(t *testing.T) {
	events := []LogEvent{{Seq: 1, Type: EventParkPermission, Decision: "allow"}}
	if got := RenderRounds(events); got != "" {
		t.Errorf("RenderRounds = %q, want empty", got)
	}
	if got := RenderRounds(nil); got != "" {
		t.Errorf("RenderRounds(nil) = %q, want empty", got)
	}
}

func TestRenderRounds_OpenRoundShowsQuestionBlockAndHandoff(t *testing.T) {
	events := []LogEvent{{
		Seq: 1, Type: EventParkQuestion, Summary: "Pick a store?", Text: "Details here.",
		Block: "type: yes-no", Handoff: "Resume at step 3.",
	}}
	want := "Park rounds\n" +
		"1. open: Pick a store?\n" +
		"   Details here.\n" +
		"   type: yes-no\n" +
		"   Handoff:\n" +
		"   Resume at step 3.\n"
	if got := RenderRounds(events); got != want {
		t.Errorf("RenderRounds =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderRounds_AnsweredRoundShowsAnswer(t *testing.T) {
	events := []LogEvent{
		{Seq: 1, Type: EventParkQuestion, Summary: "Which?", Text: "body"},
		{Seq: 2, Type: EventParkAnswer, Question: 1, Choice: "a", Note: "go"},
	}
	want := "Park rounds\n" +
		"1. answered: Which?\n" +
		"   body\n" +
		"   answer: a - go\n"
	if got := RenderRounds(events); got != want {
		t.Errorf("RenderRounds =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderRounds_ClosedRoundShowsReason(t *testing.T) {
	events := []LogEvent{
		{Seq: 1, Type: EventParkQuestion, Summary: "Which?"},
		{Seq: 2, Type: EventParkClosed, Question: 1, Reason: "ticket dropped"},
	}
	want := "Park rounds\n1. closed: Which?\n   closed: ticket dropped\n"
	if got := RenderRounds(events); got != want {
		t.Errorf("RenderRounds =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderRounds_ThreeRoundsEarlierOnesAreOneLine(t *testing.T) {
	events := []LogEvent{
		{Seq: 1, Type: EventParkQuestion, Summary: "First?", Text: "first detail"},
		{Seq: 2, Type: EventParkAnswer, Question: 1, Choice: "yes"},
		{Seq: 3, Type: EventParkQuestion, Summary: "Second?", Text: "second detail"},
		{Seq: 4, Type: EventParkClosed, Question: 3},
		{Seq: 5, Type: EventParkQuestion, Summary: "Third?", Text: "third detail"},
	}
	want := "Park rounds\n" +
		"1. answered: First? -> answer: yes\n" +
		"2. closed: Second? -> closed\n" +
		"3. open: Third?\n" +
		"   third detail\n"
	got := RenderRounds(events)
	if got != want {
		t.Errorf("RenderRounds =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "first detail") || strings.Contains(got, "second detail") {
		t.Errorf("earlier rounds must be one line, got:\n%s", got)
	}
}
