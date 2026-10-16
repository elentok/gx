package app

import (
	"strconv"
	"sync/atomic"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/version"
)

// aboutState is what the help dialog's About section reads. Model copies share
// it, so the provider registered once in New always sees the latest conn.
type aboutState struct {
	conn     atomic.Pointer[ServerConn]
	terminal ui.Terminal
}

func newAboutState(terminal ui.Terminal) *aboutState {
	a := &aboutState{terminal: terminal}
	help.SetAboutProvider(a.lines)
	return a
}

func (a *aboutState) lines() []help.AboutLine {
	conn := ServerConn{}
	if c := a.conn.Load(); c != nil {
		conn = *c
	}
	lines := []help.AboutLine{
		{Label: "version", Value: version.Get()},
		{Label: "server", Value: conn.words()},
	}
	if conn.PID > 0 {
		lines = append(lines, help.AboutLine{Label: "pid", Value: strconv.Itoa(conn.PID)})
	}
	split := a.terminal.String()
	if split == "" {
		split = "none"
	}
	return append(lines, help.AboutLine{Label: "split", Value: split})
}

// setServerConn is the one writer of the connection state.
func (m *Model) setServerConn(c ServerConn) {
	m.serverConn = c
	m.about.conn.Store(&c)
}

func (c ServerConn) words() string {
	switch c.State {
	case ServerUp:
		return "running"
	case ServerDown:
		return "down"
	case ServerReadOnly:
		return "read-only — restart"
	case ServerHerdrUnavailable:
		return "herdr unavailable"
	}
	return "connecting"
}
