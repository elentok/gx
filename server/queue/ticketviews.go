package queue

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TicketRef is a ticket found in the store, with where it lives.
type TicketRef struct {
	Addr       tickets.Address
	ProjectDir string
	Repo       string
	Ticket     tickets.Ticket
	Epic       tickets.Epic
}

// Root is the root the ticket belongs to.
func (r TicketRef) Root() RootRef { return RootOf(r.Addr) }

// Address is the canonical address of sibling ticket t in the same epic.
func (r TicketRef) Address(t tickets.Ticket) string {
	return tickets.Address{Project: r.Addr.Project, Epic: r.Addr.Epic, ID: t.Identifier}.String()
}

// TicketAddress is the canonical address of ticket id under the root.
func (r RootRef) TicketAddress(id string) string {
	return tickets.Address{Project: r.Project, Epic: r.Epic, ID: id}.String()
}

// EpicAddress is the "project:epic" part of a ticket address, whatever project
// the session itself runs in.
func EpicAddress(addr string) string {
	epicPart, _, _ := strings.Cut(addr, "/")
	return epicPart
}

// MapProgress is a map's current Progress step. It comes from the epic's
// design_step alone: status done always shows Done, and a missing or unknown
// field shows Chart. It never looks at files or branch refs.
func MapProgress(m tickets.Epic) wire.ProgressStep {
	if m.Status == string(schema.StatusDone) {
		return wire.ProgressDone
	}
	if step, ok := tickets.ParseDesignStep(m.DesignStep); ok {
		return step
	}
	return wire.ProgressChart
}

// AnswerState is the token a client sends back: it covers the latest
// question's seq and the ticket's pause class, so an answer to a question that
// was replaced, closed or re-paused is stale.
func AnswerState(q tickets.LogEvent, class schema.PauseClass) string {
	return fmt.Sprintf("%d:%s", q.Seq, class)
}

// ReadLogLenient reads a ticket's log for the snapshot, where an unreadable log
// only costs the question details.
func ReadLogLenient(t tickets.Ticket) []tickets.LogEvent {
	evs, _ := tickets.ReadLog(t.Path)
	return evs
}
