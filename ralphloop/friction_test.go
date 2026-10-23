package ralphloop

import (
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestFriction(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	run := func(s int, typ, kind string) Event {
		return Event{Time: at(s), Type: typ, Ticket: "03", Kind: kind}
	}
	q := func(seq, s int, kind string) tickets.LogEvent {
		return tickets.LogEvent{Seq: seq, TS: at(s), Type: tickets.EventPauseQuestion, PauseKind: kind}
	}
	closed := func(seq, question, s int, source string) tickets.LogEvent {
		return tickets.LogEvent{Seq: seq, TS: at(s), Type: tickets.EventPauseClosed, Question: question, Source: source}
	}

	tests := []struct {
		name string
		evs  []Event
		log  []tickets.LogEvent
		want schema.FrictionSummary
	}{
		{
			name: "one round is one pause, ended by a person",
			evs:  []Event{run(0, "paused", "zero-commit"), run(5, "paused", "zero-commit")},
			log:  []tickets.LogEvent{q(1, 0, "zero-commit"), closed(2, 1, 90, "web")},
			want: schema.FrictionSummary{
				PausesByReason:  map[string]int{"zero-commit": 1},
				PauseEndings:    map[string]int{"web": 1},
				TotalPausedSecs: 90,
				PersonPauseSecs: []int{90},
			},
		},
		{
			name: "gx-ended round counts in the total only",
			log:  []tickets.LogEvent{q(1, 0, "blocked-pane"), closed(2, 1, 30, "gx")},
			want: schema.FrictionSummary{
				PausesByReason:  map[string]int{"blocked-pane": 1},
				PauseEndings:    map[string]int{"gx": 1},
				TotalPausedSecs: 30,
			},
		},
		{
			name: "round without kind or source, and an open round",
			log:  []tickets.LogEvent{q(1, 0, ""), closed(2, 1, 10, ""), q(3, 20, "budget-killed")},
			want: schema.FrictionSummary{
				PausesByReason:  map[string]int{"unknown": 1, "budget-killed": 1},
				PauseEndings:    map[string]int{"unknown": 1},
				TotalPausedSecs: 10,
			},
		},
		{
			name: "legacy kind name maps to the current one",
			log:  []tickets.LogEvent{q(1, 0, "land-conflict"), closed(2, 1, 4, "cli")},
			want: schema.FrictionSummary{
				PausesByReason:  map[string]int{"merge-conflict": 1},
				PauseEndings:    map[string]int{"cli": 1},
				TotalPausedSecs: 4,
				PersonPauseSecs: []int{4},
			},
		},
		{
			name: "old data: repeated paused lines are one spell",
			evs: []Event{
				run(0, "iteration-started", ""),
				run(10, "paused", "self-reported"),
				run(20, "paused", "self-reported"),
				run(60, "iteration-started", ""),
				run(70, "paused", ""),
				run(80, "paused", "budget-killed"),
			},
			want: schema.FrictionSummary{
				PausesByReason:  map[string]int{"self-reported": 1, "unknown": 1},
				PauseEndings:    map[string]int{"unknown": 2},
				TotalPausedSecs: 50,
			},
		},
		{
			name: "old data: other tickets are ignored",
			evs:  []Event{{Time: at(0), Type: "paused", Ticket: "04", Kind: "zero-commit"}},
			want: schema.FrictionSummary{},
		},
		{
			name: "run events: counted series",
			evs: []Event{
				run(0, "conflict-hit", ""),
				run(1, "ticket-started-over", ""),
				run(2, "claim-reopened", ""),
				run(3, "manual-merge", ""),
				run(4, "recovery-escalated", "spinning"),
				run(5, "paused-rate-limit", ""),
				run(6, "launch-failed", "agent_name_taken"),
				run(7, "launch-failed", "agent_name_taken"),
			},
			want: schema.FrictionSummary{RunEvents: map[string]int{
				"conflict-hit": 1, "ticket-started-over": 2, "manual-merge": 1,
				"recovery-escalated": 1, "paused-rate-limit": 1, "launch-failed": 2,
			}},
		},
		{
			name: "run events: left-out events",
			evs: []Event{
				run(0, "nudged", ""), run(1, "stall-nudged", ""), run(2, "prompt-nudged", ""),
				run(3, "deadlocked", "blocked-cycle"), run(4, "paused-smart-zone", ""),
				run(5, "late-finish-adopted", ""), run(6, "human-relaunch", ""),
				run(7, "recovery-applied", "spinning"),
			},
			want: schema.FrictionSummary{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Friction("03", tt.evs, tt.log); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
