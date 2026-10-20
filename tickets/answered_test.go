package tickets_test

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestAnswered(t *testing.T) {
	q := tickets.LogEvent{Type: tickets.EventParkQuestion, Seq: 1}
	answer := tickets.LogEvent{Type: tickets.EventParkAnswer, Seq: 2, Question: 1}
	closed := tickets.LogEvent{Type: tickets.EventParkClosed, Seq: 2, Question: 1}
	q2 := tickets.LogEvent{Type: tickets.EventParkQuestion, Seq: 3}
	cases := []struct {
		name   string
		status schema.Status
		evs    []tickets.LogEvent
		want   bool
	}{
		{"open and answered", schema.StatusOpen, []tickets.LogEvent{q, answer}, true},
		{"claimed clears it", schema.StatusClaimed, []tickets.LogEvent{q, answer}, false},
		{"open, still asking", schema.StatusOpen, []tickets.LogEvent{q}, false},
		{"open, closed unanswered", schema.StatusOpen, []tickets.LogEvent{q, closed}, false},
		{"newer question unanswered", schema.StatusOpen, []tickets.LogEvent{q, answer, q2}, false},
		{"no question", schema.StatusOpen, nil, false},
	}
	for _, c := range cases {
		if got := tickets.Answered(c.status, c.evs); got != c.want {
			t.Errorf("%s: Answered = %v, want %v", c.name, got, c.want)
		}
	}
}
