package ticketlog_test

import (
	"testing"

	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

func TestAnswered(t *testing.T) {
	q := ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, Seq: 1}
	answer := ticketlog.LogEvent{Type: ticketlog.EventPauseAnswer, Seq: 2, Question: 1}
	closed := ticketlog.LogEvent{Type: ticketlog.EventPauseClosed, Seq: 2, Question: 1}
	q2 := ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, Seq: 3}
	cases := []struct {
		name   string
		status schema.Status
		evs    []ticketlog.LogEvent
		want   bool
	}{
		{"open and answered", schema.StatusOpen, []ticketlog.LogEvent{q, answer}, true},
		{"claimed clears it", schema.StatusClaimed, []ticketlog.LogEvent{q, answer}, false},
		{"open, still asking", schema.StatusOpen, []ticketlog.LogEvent{q}, false},
		{"open, closed unanswered", schema.StatusOpen, []ticketlog.LogEvent{q, closed}, false},
		{"newer question unanswered", schema.StatusOpen, []ticketlog.LogEvent{q, answer, q2}, false},
		{"no question", schema.StatusOpen, nil, false},
	}
	for _, c := range cases {
		if got := ticketlog.Answered(c.status, c.evs); got != c.want {
			t.Errorf("%s: Answered = %v, want %v", c.name, got, c.want)
		}
	}
}
