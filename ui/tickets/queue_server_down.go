package tickets

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
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

func cmdProbeRequested() tea.Msg { return ProbeRequestedMsg{} }

// serverDown reports the down banner state: the link the shell last delivered.
func (m QueueModel) serverDown() bool { return m.link == ServerLinkDown }

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
