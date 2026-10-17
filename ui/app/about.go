package app

import (
	"strconv"
	"sync/atomic"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/version"
)

// aboutLines is the help dialog's About provider. It reads the conn cell
// itself rather than a Model copy, so a page built before a handshake still
// shows the latest server state.
func aboutLines(conn *atomic.Pointer[ServerConn], terminal ui.Terminal) func() []ui.AboutLine {
	return func() []ui.AboutLine {
		c := loadConn(conn)
		lines := []ui.AboutLine{
			{Label: "version", Value: version.Get()},
			{Label: "server", Value: c.words()},
		}
		if c.PID > 0 {
			lines = append(lines, ui.AboutLine{Label: "pid", Value: strconv.Itoa(c.PID)})
		}
		split := terminal.String()
		if split == "" {
			split = "none"
		}
		return append(lines, ui.AboutLine{Label: "split", Value: split})
	}
}

func loadConn(conn *atomic.Pointer[ServerConn]) ServerConn {
	if c := conn.Load(); c != nil {
		return *c
	}
	return ServerConn{}
}

func (m Model) serverConn() ServerConn { return loadConn(m.conn) }

// setServerConn is the one writer of the connection state.
func (m *Model) setServerConn(c ServerConn) { m.conn.Store(&c) }

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
