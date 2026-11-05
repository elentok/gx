package rowfmt

import (
	"strings"

	"github.com/elentok/gx/tickets"
)

// TicketMatchesQuery is the search rule for a ticket row, shared by both
// tabs: case-insensitive substring over its "<id> <title>" row text and its
// rendered status word. An epic-name match is EpicMatchesQuery's job, so it
// lands on the epic row rather than on every ticket inside it. q must already
// be lowercased and trimmed.
func TicketMatchesQuery(epic tickets.Epic, t tickets.Ticket, q string) bool {
	haystack := t.DisplayNumber() + " " + t.Title + " " + TicketStatus(epic, t).Word()
	return strings.Contains(strings.ToLower(haystack), q)
}

// EpicMatchesQuery is the search rule for an epic row, shared by both tabs.
// An epic's Name is both its slug and its displayed title. q must already be
// lowercased and trimmed.
func EpicMatchesQuery(epic tickets.Epic, q string) bool {
	return strings.Contains(strings.ToLower(epic.Name), q)
}
