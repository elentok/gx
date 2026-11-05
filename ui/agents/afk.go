package agents

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/notify"
)

type afkMsg struct {
	result wire.AFKResult
	err    error
}

func (a sessionActions) toggleAFK() tea.Cmd {
	if a.client == nil {
		return nil
	}
	sessions := a.sessionAPI
	return func() tea.Msg {
		res, err := sessions.SetAFK(context.Background(), nil)
		return afkMsg{result: res, err: err}
	}
}

// updateAFK shows the new mode at once, ahead of the snapshot that follows.
func updateAFK(msg afkMsg, afk bool) (bool, tea.Cmd) {
	switch {
	case msg.err != nil:
		return afk, notify.Error("AFK failed: " + msg.err.Error())
	case msg.result.Refused:
		return afk, notify.Error("AFK refused: " + msg.result.Reason)
	}
	return msg.result.AFK, nil
}

// WithFocusSession selects and focuses session id once a snapshot lists it.
func (m Model) WithFocusSession(id string) Model {
	m.pendingSelect = id
	return m
}
