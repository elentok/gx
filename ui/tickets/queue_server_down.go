package tickets

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
)

// ServerDownMsg tells the Queue tab the server stopped answering; ServerUpMsg
// that it answers again. The app shell delivers them from the connection state.
type (
	ServerDownMsg struct{}
	ServerUpMsg   struct{}
)

type (
	queueServerRetryMsg     struct{}
	queueServerStartedMsg   struct{ err error }
	queueServerStartConfirm struct{}
)

const queueServerDownBanner = "server down — s to start"

// WithServerLink puts the Queue tab in server mode: api is probed in the
// background while the server is down, and start launches it on "s".
func (m QueueModel) WithServerLink(api ServerAPI, start func(context.Context) error) QueueModel {
	m.serverAPI = api
	m.serverStart = start
	return m
}

func (m QueueModel) cmdServerRetryLater() tea.Cmd {
	return tea.Tick(serverReconnectDelay, func(time.Time) tea.Msg { return queueServerRetryMsg{} })
}

func (m QueueModel) cmdServerProbe() tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		if _, err := api.Snapshot(context.Background()); err != nil {
			return queueServerRetryMsg{}
		}
		return ServerUpMsg{}
	}
}

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
		return m, m.cmdServerRetryLater(), true
	case queueServerRetryMsg:
		if !m.serverDown {
			return m, nil, true
		}
		return m, tea.Batch(m.cmdServerProbe(), m.cmdServerRetryLater()), true
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
		return m, m.cmdServerProbe(), true
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
