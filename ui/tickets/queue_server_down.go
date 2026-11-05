package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/tickets/tabapi"
)

type (
	queueServerStartedMsg   struct{ err error }
	queueServerStartConfirm struct{}
)

// WithServerLink puts the Queue tab in server mode: rows come from the state
// the app shell delivers, and start launches the server on "s".
func (m QueueModel) WithServerLink(api tabapi.ServerAPI, start func(context.Context) error) QueueModel {
	m.server.serverAPI = api
	m.server.serverStart = start
	return m
}

// WithSessions gives the tab the session sub-client; see Model.WithSessions.
func (m QueueModel) WithSessions(s tabapi.SessionsAPI) QueueModel {
	m.sessions = s
	return m
}

// WithQueue gives the tab the queue sub-client; see Model.WithQueue.
func (m QueueModel) WithQueue(q tabapi.QueueAPI) QueueModel {
	m.srv.queue = q
	return m
}

// WithReads gives the tab the reads sub-client; see Model.WithReads.
func (m QueueModel) WithReads(r tabapi.ReadsAPI) QueueModel {
	m.srv.reads = r
	return m
}

// WithAdmin gives the tab the admin sub-client; see Model.WithAdmin.
func (m QueueModel) WithAdmin(a tabapi.AdminAPI) QueueModel {
	m.srv.admin = a
	return m
}

// WithTickets gives the tab the tickets sub-client; see Model.WithTickets.
func (m QueueModel) WithTickets(t tabapi.TicketsAPI) QueueModel {
	m.srv.tickets = t
	return m
}

func cmdProbeRequested() tea.Msg { return tabapi.ProbeRequestedMsg{} }
