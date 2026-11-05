package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

type (
	queueServerStartedMsg   struct{ err error }
	queueServerStartConfirm struct{}
)

const queueServerDownBanner = "server not running — s to start"

// WithServerLink puts the Queue tab in server mode: rows come from the state
// the app shell delivers, and start launches the server on "s".
func (m QueueModel) WithServerLink(api tabapi.ServerAPI, start func(context.Context) error) QueueModel {
	m.serverAPI = api
	m.serverStart = start
	return m
}

// WithSessions gives the tab the session sub-client; see Model.WithSessions.
func (m QueueModel) WithSessions(s tabapi.SessionsAPI) QueueModel {
	m.sessions = s
	return m
}

// WithQueue gives the tab the queue sub-client; see Model.WithQueue.
func (m QueueModel) WithQueue(q tabapi.QueueAPI) QueueModel {
	m.queue = q
	return m
}

// WithReads gives the tab the reads sub-client; see Model.WithReads.
func (m QueueModel) WithReads(r tabapi.ReadsAPI) QueueModel {
	m.reads = r
	return m
}

// WithAdmin gives the tab the admin sub-client; see Model.WithAdmin.
func (m QueueModel) WithAdmin(a tabapi.AdminAPI) QueueModel {
	m.admin = a
	return m
}

// WithTickets gives the tab the tickets sub-client; see Model.WithTickets.
func (m QueueModel) WithTickets(t tabapi.TicketsAPI) QueueModel {
	m.tickets = t
	return m
}

func cmdProbeRequested() tea.Msg { return tabapi.ProbeRequestedMsg{} }

// serverDown reports the down banner state: the link the shell last delivered.
func (m QueueModel) serverDown() bool { return m.link == tabapi.ServerLinkDown }

// updateServerDown handles the server-start messages; ok is false for any other msg.
func (m QueueModel) updateServerDown(msg tea.Msg) (QueueModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case queueServerStartConfirm:
		start := m.serverStart
		return m, func() tea.Msg { return queueServerStartedMsg{err: start(context.Background())} }, true
	case queueServerStartedMsg:
		if msg.err != nil {
			return m, notify.Error("start server: " + msg.err.Error()), true
		}
		return m, cmdProbeRequested, true
	}
	return m, nil, false
}

// openServerStartConfirm asks before starting the server; a no-op when the
// server is up or the tab has no way to start it.
func (m QueueModel) openServerStartConfirm() QueueModel {
	if !m.serverDown() || m.serverStart == nil {
		return m
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "server is down — start it now?",
		AcceptCmd: func() tea.Msg { return queueServerStartConfirm{} },
	})
	return m
}
