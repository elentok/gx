package server

import (
	"slices"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
	"github.com/elentok/gx/tickets/ticketmove"
)

// startDesignHost is the Server as Start Design sees it.
type startDesignHost struct{ sessionHost }

func (h startDesignHost) Epics(project string) ([]tickets.Epic, error) {
	return h.projects.Epics(project)
}
func (h startDesignHost) ProjectDir(project string) (string, error) { return h.projects.Dir(project) }
func (h startDesignHost) ConversationsDir() string                  { return h.cfg.ConversationsDir }
func (h startDesignHost) LaunchHarness(project string) string {
	return h.projects.LaunchHarness(project)
}
func (h startDesignHost) TrunkCheckout(project string) string {
	return ticketgit.TrunkCheckout(h.cfg.TicketStore, project)
}

func (h startDesignHost) CreateSession(req wire.CreateSessionRequest) (wire.SessionResult, error) {
	return h.sessions.CreateSession(req, sessions.SessionCtx{})
}

// Active refuses a ticket in the queue. It does not look at running
// iterations, which the CLI check also reads.
func (h startDesignHost) Active(a tickets.Address) error {
	queued := slices.ContainsFunc(h.queue.List(), func(i wire.QueueItem) bool { return i.Address == a.String() })
	if queued {
		return ticketmove.MoveRefusal(tickets.CodeMoveActive, "ticket %s is queued or live on the server", a)
	}
	return nil
}
