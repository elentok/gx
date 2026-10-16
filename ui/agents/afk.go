package agents

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/notify"
)

type afkMsg struct {
	result server.AFKResult
	err    error
}

func (m Model) toggleAFKCmd() tea.Cmd {
	if m.client == nil {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		res, err := client.SetAFK(context.Background(), nil)
		return afkMsg{result: res, err: err}
	}
}

// updateAFK shows the new mode at once, ahead of the snapshot that follows.
func (m Model) updateAFK(msg afkMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		return m, notify.Error("AFK failed: " + msg.err.Error())
	case msg.result.Refused:
		return m, notify.Error("AFK refused: " + msg.result.Reason)
	}
	m.afk = msg.result.AFK
	return m, nil
}

// WithFocusSession selects and focuses session id once a snapshot lists it.
func (m Model) WithFocusSession(id string) Model {
	m.pendingSelect = id
	return m
}
