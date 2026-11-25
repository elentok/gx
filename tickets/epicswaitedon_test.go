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
		{"epic-level", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", BlockedBy: []string{"a"}, Tickets: []Ticket{open()}}}, []string{"a"}},
		{"terminal dependent", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", BlockedBy: []string{"a"}, Tickets: []Ticket{done()}}}, nil},
		{"no blockers", []Epic{{Name: "a", Tickets: doneTicket()}, {Name: "d", Tickets: []Ticket{open()}}}, nil},
	} {
		got := slices.Sorted(maps.Keys(EpicsWaitedOn("p", tc.epics)))
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
