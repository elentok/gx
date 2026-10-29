package tickets

import (
	"fmt"
	"slices"
	"testing"
)

// A loaded epic builds the fork-children map once for every per-ticket status
// call, and rebuilds it when its tickets change.
func TestRenderedStatus_BuildsForkChildrenOncePerEpic(t *testing.T) {
	root := "01"
	e := Epic{forks: &forkCache{}, Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "01a", Status: "open", Parent: &root},
		{Number: 3, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	before := forkChildrenBuilds.Load()
	for _, tk := range e.Tickets {
		e.RenderedStatus(tk)
		e.IsPaused(tk)
		e.UnresolvedBlockers(tk)
	}
	if got := forkChildrenBuilds.Load() - before; got != 1 {
		t.Errorf("fork-children builds = %d, want 1", got)
	}
	if got := e.RenderedStatus(e.Tickets[0]); got != StatusWaitingForChildren {
		t.Errorf("fork parent status = %v, want waiting-for-children", got)
	}

	e.Tickets = append(slices.Clone(e.Tickets), Ticket{Number: 4, Identifier: "01b", Status: "open", Parent: &root})
	if got := len(e.forkChildren()[ticketKey(e.Tickets[0])]); got != 2 {
		t.Errorf("children of 01 after append = %d, want 2", got)
	}
}

func codeReviewEpic(n int) Epic {
	e := Epic{forks: &forkCache{}}
	for i := 1; i <= n; i++ {
		tk := Ticket{Number: i, Identifier: fmt.Sprintf("%02d", i), Status: "open"}
		if i == n {
			tk.Type = typeCodeReview
		}
		e.Tickets = append(e.Tickets, tk)
	}
	return e
}

// A code-review ticket's blocker expansion looks tickets up by number for every
// other ticket; the index behind that is built once per epic, not per lookup.
func TestRenderedStatus_BuildsTicketIndexOncePerEpic(t *testing.T) {
	e := codeReviewEpic(70)
	before := ticketIndexBuilds.Load()
	for _, tk := range e.Tickets {
		e.RenderedStatus(tk)
	}
	if got := ticketIndexBuilds.Load() - before; got != 1 {
		t.Errorf("ticket-index builds = %d, want 1", got)
	}
}

func BenchmarkRenderedStatus_CodeReviewEpic(b *testing.B) {
	e := codeReviewEpic(70)
	for b.Loop() {
		for _, tk := range e.Tickets {
			e.RenderedStatus(tk)
		}
	}
}

// OpenCount must build the fork-children map once per pass, not once per
// ticket: allocations grow linearly with the epic, not quadratically.
func TestOpenCount_AllocationsStayLinearInEpicSize(t *testing.T) {
	build := func(n int) Epic {
		e := Epic{forks: &forkCache{}}
		for i := 1; i <= n; i++ {
			tk := Ticket{Number: i, Identifier: fmt.Sprintf("%02d", i), Status: "done"}
			if i > 1 {
				parent := fmt.Sprintf("%02d", i-1)
				tk.Parent = &parent
			}
			e.Tickets = append(e.Tickets, tk)
		}
		return e
	}
	small, large := build(20), build(80)
	allocs := func(e Epic) float64 { return testing.AllocsPerRun(5, func() { e.OpenCount() }) }
	a, b := allocs(small), allocs(large)
	// 4x the tickets: linear is about 4x the allocations, quadratic about 16x.
	if b > a*8 {
		t.Errorf("allocs grew %.0f -> %.0f for 4x tickets, want roughly linear", a, b)
	}
}
