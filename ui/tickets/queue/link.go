package queue

import (
	"context"

	"github.com/elentok/gx/ui/tickets/tabapi"
)

// WithServerLink puts the Queue tab in server mode: rows come from the state
// the app shell delivers, and start launches the server on "s".
func (m QueueModel) WithServerLink(api tabapi.ServerAPI, start func(context.Context) error) QueueModel {
	m.server.ServerAPI = api
	m.server.ServerStart = start
	return m
}

// WithSessions gives the tab the session sub-client; see Model.WithSessions.
func (m QueueModel) WithSessions(s tabapi.SessionsAPI) QueueModel {
	m.sessions = s
	return m
}

// WithQueue gives the tab the queue sub-client; see Model.WithQueue.
func (m QueueModel) WithQueue(q tabapi.QueueAPI) QueueModel {
	m.srv.Queue = q
	return m
}

// WithReads gives the tab the reads sub-client; see Model.WithReads.
func (m QueueModel) WithReads(r tabapi.ReadsAPI) QueueModel {
	m.srv.Reads = r
	return m
}

// WithAdmin gives the tab the admin sub-client; see Model.WithAdmin.
func (m QueueModel) WithAdmin(a tabapi.AdminAPI) QueueModel {
	m.srv.Admin = a
	return m
}

// WithTickets gives the tab the tickets sub-client; see Model.WithTickets.
func (m QueueModel) WithTickets(t tabapi.TicketsAPI) QueueModel {
	m.srv.Tickets = t
	return m
}
