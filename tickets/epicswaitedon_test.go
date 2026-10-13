package tickets

import (
	"maps"
	"slices"
	"testing"
)

func TestEpicsWaitedOn(t *testing.T) {
	open := func(blockedBy ...string) Ticket {
		return Ticket{Number: 1, Identifier: "01", Status: "open", BlockedBy: blockedBy}
	}
	done := func(blockedBy ...string) Ticket {
		t := open(blockedBy...)
		t.Status = "done"
		return t
	}
	for _, tc := range []struct {
		name  string
		epics []Epic
		want  []string
	}{
		{"whole epic", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", Tickets: []Ticket{open("a")}}}, []string{"a"}},
		{"widened ticket", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", Tickets: []Ticket{open("a/01")}}}, []string{"a"}},
		{"epic-level", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", BlockedBy: []string{"a"}, Tickets: []Ticket{open()}}}, []string{"a"}},
		{"terminal dependent", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", Tickets: []Ticket{done("a")}}}, nil},
		{"commitless ticket stays a ticket", []Epic{
			{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "done", Commitless: true}}},
			{Name: "d", Tickets: []Ticket{open("a/01")}},
		}, nil},
		{"no blockers", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", Tickets: []Ticket{open()}}}, nil},
	} {
		got := slices.Sorted(maps.Keys(EpicsWaitedOn("p", tc.epics)))
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
