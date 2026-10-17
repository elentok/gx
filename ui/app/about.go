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

// words reuses the title-row badge's words so the two can't drift; the badge
// has none for the up and unknown states.
func (c ServerConn) words() string {
	if c.State == ServerUp {
		return "running"
	}
	if b, ok := c.badge(ui.IconSet{}); ok {
		return b.full
	}
	return "connecting"
}
