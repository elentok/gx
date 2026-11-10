package prstate

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func tk(id, status string, typ schema.TicketType) tickets.Ticket {
	return tickets.Ticket{Identifier: id, Status: status, Type: string(typ)}
}

func TestOf(t *testing.T) {
	t.Parallel()
	done := tk("01", "done", schema.TypeImplement)
	paused := tk("03", "paused", schema.TypeCIFix)
	paused.PauseReason = schema.PauseManual
	merged := time.Unix(100, 0)
	cases := []struct {
		name     string
		epic     tickets.Epic
		delivery string
		want     wire.EpicState
	}{
		{"complete in a pr project", tickets.Epic{Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStateAwaitingPR},
		{"done pr-text ticket does not block", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "done", schema.TypePRText)}}, "pr", wire.EpicStateAwaitingPR},
		{"cancelled ticket does not block", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "cancelled", schema.TypeImplement)}}, "pr", wire.EpicStateAwaitingPR},
		{"merge project", tickets.Epic{Tickets: []tickets.Ticket{done}}, "merge", ""},
		{"open verify ticket blocks", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "open", schema.TypeVerify)}}, "pr", ""},
		{"incomplete epic", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "open", schema.TypeImplement)}}, "pr", ""},
		{"merged epic", tickets.Epic{MergedAt: merged, Tickets: []tickets.Ticket{done}}, "pr", ""},
		{"map epic", tickets.Epic{IsMap: true, Tickets: []tickets.Ticket{done}}, "pr", ""},
		{"returned after a closed PR", tickets.Epic{Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStateAwaitingPR},
		{"pr open in a pr project", tickets.Epic{PRNumber: 7, Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStatePROpen},
		{"pr open after delivery went back to merge", tickets.Epic{PRNumber: 7, Tickets: []tickets.Ticket{done}}, "merge", wire.EpicStatePROpen},
		{"open pr-text", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "open", schema.TypePRText)}}, "pr", wire.EpicStateWritingPRText},
		{"claimed pr-text", tickets.Epic{Tickets: []tickets.Ticket{done, tk("02", "claimed", schema.TypePRText)}}, "merge", wire.EpicStateWritingPRText},
		{"open ci-fix", tickets.Epic{PRNumber: 7, Tickets: []tickets.Ticket{done, tk("02", "open", schema.TypeCIFix)}}, "pr", wire.EpicStateFixingCI},
		{"paused ci-fix falls through", tickets.Epic{Tickets: []tickets.Ticket{done, paused}}, "pr", ""},
		{"ci-fix beside other work", tickets.Epic{Tickets: []tickets.Ticket{tk("01", "open", schema.TypeImplement), tk("02", "open", schema.TypeCIFix)}}, "pr", ""},
		{"ci-failing stored", tickets.Epic{PRNumber: 7, MergeBlockKind: "ci-failing", MergeBlockReason: "x", Tickets: []tickets.Ticket{done}}, "pr", ""},
		{"pr-text-failed stored", tickets.Epic{MergeBlockKind: "pr-text-failed", MergeBlockReason: "x", Tickets: []tickets.Ticket{done}}, "pr", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Of(tc.epic, tc.delivery); got != tc.want {
				t.Fatalf("Of = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBlocked(t *testing.T) {
	t.Parallel()
	done := tk("01", "done", schema.TypeImplement)
	open := tk("02", "open", schema.TypeImplement)
	cases := []struct {
		name string
		epic tickets.Epic
		want bool
	}{
		{"ci-failing", tickets.Epic{MergeBlockKind: "ci-failing", Tickets: []tickets.Ticket{done}}, true},
		{"pr-text-failed", tickets.Epic{MergeBlockKind: "pr-text-failed", Tickets: []tickets.Ticket{done}}, true},
		{"merge kind", tickets.Epic{MergeBlockKind: "needs-rebase", Tickets: []tickets.Ticket{done}}, false},
		{"incomplete", tickets.Epic{MergeBlockKind: "ci-failing", Tickets: []tickets.Ticket{done, open}}, false},
		{"merged", tickets.Epic{MergeBlockKind: "ci-failing", MergedAt: time.Unix(1, 0), Tickets: []tickets.Ticket{done}}, false},
	}
	for _, tc := range cases {
		if got := Blocked(tc.epic); got != tc.want {
			t.Errorf("%s: Blocked = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCIFixAttempt(t *testing.T) {
	t.Parallel()
	reset := time.Unix(100, 0)
	old, fresh := tk("01", "done", schema.TypeCIFix), tk("02", "done", schema.TypeCIFix)
	old.CreatedAt, fresh.CreatedAt = reset.Add(-time.Hour), reset.Add(time.Hour)
	e := tickets.Epic{Tickets: []tickets.Ticket{old, fresh, tk("03", "done", schema.TypeImplement)}}
	if got := CIFixAttempt(e, reset); got != 1 {
		t.Errorf("since reset = %d, want 1", got)
	}
	if got := CIFixAttempt(e, time.Time{}); got != 2 {
		t.Errorf("no reset = %d, want 2", got)
	}
}
