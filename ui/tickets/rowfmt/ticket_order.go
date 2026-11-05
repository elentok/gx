package rowfmt

import (
	"sort"

	"github.com/elentok/gx/tickets"
)

// SortedTicketIndexes orders epic.Tickets' indexes in plan order — ticket
// number ascending, so the list reads as the epic's intended order of
// execution and a ticket (done or otherwise) never jumps out of its place
// once it finishes. Lettered siblings sharing a Number (e.g. 04,
// 04a, 04b) tie-break on DisplayNumber, so the original sorts before its
// replacements in filename order.
func SortedTicketIndexes(epic tickets.Epic) []int {
	indexes := make([]int, len(epic.Tickets))
	for i := range indexes {
		indexes[i] = i
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, b := epic.Tickets[indexes[i]], epic.Tickets[indexes[j]]
		if a.Number != b.Number {
			return a.Number < b.Number
		}
		return a.DisplayNumber() < b.DisplayNumber()
	})
	return indexes
}
