package triage

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// OldEpicNote is the Notes text of an area epic an earlier Triage made.
const OldEpicNote = "Triaged from follow-ups"

// IsOldEpic: a map, still charting, whose Notes say it was triaged from
// follow-ups.
func IsOldEpic(e tickets.Epic) bool {
	return e.IsCharting() && strings.Contains(schema.Section(e.Body, "Notes"), OldEpicNote)
}

// OldEpicTickets sorts an old epic's tickets by what Triage does with them.
type OldEpicTickets struct {
	// Move: open and draft tickets, addresses. They move into an area.
	Move []string
	// Claimed: addresses. A claimed ticket holds the archive.
	Claimed []string
	// Live counts tickets that are neither done nor cancelled.
	Live int
}

// ClassifyOldEpic is the one place that decides which of e's tickets move,
// which hold the archive and which are still live.
func ClassifyOldEpic(project string, e tickets.Epic) OldEpicTickets {
	out := OldEpicTickets{Move: []string{}, Claimed: []string{}}
	for _, tk := range e.Tickets {
		addr := tickets.Address{Project: project, Epic: e.Name, ID: tk.Identifier}.String()
		switch s := schema.Status(tk.Status); s {
		case schema.StatusOpen, schema.StatusDraft:
			out.Move = append(out.Move, addr)
		case schema.StatusClaimed:
			out.Claimed = append(out.Claimed, addr)
		}
		if s := schema.Status(tk.Status); s != schema.StatusDone && s != schema.StatusCancelled {
			out.Live++
		}
	}
	return out
}

// Waiting: nothing can move and a claimed ticket stays, so the Triage leaves
// the epic out.
func (o OldEpicTickets) Waiting() bool { return len(o.Move) == 0 && len(o.Claimed) > 0 }

// WaitingLine is the summary line for a waiting epic.
func (o OldEpicTickets) WaitingLine(epic string) string {
	return fmt.Sprintf("waiting: %s holds claimed %s", epic, strings.Join(o.Claimed, ", "))
}

// CanArchive: no claimed ticket stops the archive.
func (o OldEpicTickets) CanArchive() bool { return len(o.Claimed) == 0 }

// Emptied: no ticket is left that is not done or cancelled.
func (o OldEpicTickets) Emptied() bool { return o.Live == 0 }
