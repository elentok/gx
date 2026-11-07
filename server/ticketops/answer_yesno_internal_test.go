package ticketops

import (
	"testing"

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
	} {
		if got := yesNoReply(c.q, c.answer); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
