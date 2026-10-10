package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// ServerDownMsg tells the tabs the server stopped answering; ServerUpMsg that it
// answers again. The app shell delivers them from the connection state.
type (
	ServerDownMsg struct{}
	ServerUpMsg   struct{}
)

// ProbeRequestedMsg asks the app shell, which owns the connection, to check the
// server now (the Queue tab's "R" and a finished server start).
type ProbeRequestedMsg struct{}

type (
	queueServerStartedMsg   struct{ err error }
	queueServerStartConfirm struct{}
)

const queueServerDownBanner = "server down — s to start"

// WithServerLink puts the Queue tab in server mode: rows come from the state
// the app shell delivers, and start launches the server on "s".
func (m QueueModel) WithServerLink(api ServerAPI, start func(context.Context) error) QueueModel {
	m.serverAPI = api
	m.serverStart = start
	return m
}

// WithServerDown starts the tab in the down state: the server is already
// unreachable (the tab gets no ServerDownMsg crossing), or there is no client at
// all, which reads the same.
func (m QueueModel) WithServerDown() QueueModel {
	m.serverDown = true
	return m
}

func cmdProbeRequested() tea.Msg { return ProbeRequestedMsg{} }

// updateServerDown handles the server-link messages; ok is false for any other msg.
func (m QueueModel) updateServerDown(msg tea.Msg) (QueueModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ServerDownMsg:
		if m.serverDown || m.serverAPI == nil {
			return m, nil, true
		}
		m.serverDown = true
		// Whatever the queue showed is now stale; it refills on reconnect.
		m.epics, m.shared = nil, nil
		m.clampSelected()
		return m, nil, true
	case ServerUpMsg:
		if !m.serverDown {
			return m, nil, true
		}
		m.serverDown = false
		// Rows come back with the shell's next delivery; one that arrived
		// while down is applied now.
		if m.shared == nil {
			return m, nil, true
		}
		next, cmd := m.applyServerState()
		return next, cmd, true
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
	if !m.serverDown || m.serverStart == nil {
		return m
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "server is down — start it now?",
		AcceptCmd: func() tea.Msg { return queueServerStartConfirm{} },
	})
	return m
}
