package ticketops

import (
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/ticketlog"
)

func TestYesNoReply(t *testing.T) {
	yn := ticketlog.Question{Kind: ticketlog.QuestionYesNo, Yes: "keep it", No: "drop it"}
	bare := ticketlog.Question{Kind: ticketlog.QuestionYesNo}
	opts := ticketlog.Question{Kind: ticketlog.QuestionOptions, Yes: "keep it"}
	for _, c := range []struct {
		name   string
		q      ticketlog.Question
		answer string
		want   string
	}{
		{"yes echoes the yes line", yn, "yes", "Yes - keep it"},
		{"no echoes the no line", yn, "no", "No - drop it"},
		{"no consequence line stays bare", bare, "yes", "Yes"},
		{"options pass through", opts, "yes", "yes"},
		{"note alone has no option", yn, "", "no option picked"},
		{"note alone on options", opts, "", "no option picked"},
	} {
		if got := yesNoReply(c.q, c.answer); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateAnswer_NoteAlone(t *testing.T) {
	yn := ticketlog.Question{Kind: ticketlog.QuestionYesNo}
	text := ticketlog.Question{Kind: ticketlog.QuestionText}
	answer, note, bad := validateAnswer(yn, wire.QueueRequest{Note: " why not "})
	if bad != nil || answer != "" || note != "why not" {
		t.Errorf("note alone = %q, %q, %v; want empty choice, the note, no refusal", answer, note, bad)
	}
	if _, _, bad := validateAnswer(yn, wire.QueueRequest{}); bad == nil || bad.Reason != wire.ReasonEmptyAnswer {
		t.Errorf("empty choice and note = %v, want %s", bad, wire.ReasonEmptyAnswer)
	}
	if _, _, bad := validateAnswer(text, wire.QueueRequest{Note: "x"}); bad == nil || bad.Reason != wire.ReasonEmptyAnswer {
		t.Errorf("free text with only a note = %v, want %s", bad, wire.ReasonEmptyAnswer)
	}
}
