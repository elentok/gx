package tickets

import (
	"fmt"
	"testing"
)

// OpenCount must build the fork-children map once per pass, not once per
// ticket: allocations grow linearly with the epic, not quadratically.
func TestOpenCount_AllocationsStayLinearInEpicSize(t *testing.T) {
	build := func(n int) Epic {
		e := Epic{}
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
