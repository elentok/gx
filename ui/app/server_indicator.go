package app

import (
	"fmt"

	ticketsui "github.com/elentok/gx/ui/tickets"
)

// ServerState is how this TUI sees the orchestrator server.
type ServerState int

const (
	// ServerInProcess is the old mode: no server indicator is shown.
	ServerInProcess ServerState = iota
	ServerUp
	ServerDown
	// ServerReadOnly: API version mismatch; the client must not write.
	ServerReadOnly
	ServerHerdrUnavailable
)

// ServerConn is the connection state plus the server pid, when known.
type ServerConn struct {
	State ServerState
	PID   int
}

// indicator is the text shown on every tab; empty in in-process mode.
func (c ServerConn) indicator() string {
	switch c.State {
	case ServerUp:
		return fmt.Sprintf("server ● pid %d", c.PID)
	case ServerDown:
		return "server ○ down"
	case ServerReadOnly:
		return "server ◐ read-only — restart"
	case ServerHerdrUnavailable:
		return "server ⚠ herdr unavailable"
	}
	return ""
}

// link is the connection state as the Tickets tab gates its keys and live
// columns on it.
func (c ServerConn) link() ticketsui.ServerLink {
	switch c.State {
	case ServerDown:
		return ticketsui.ServerLinkDown
	case ServerReadOnly:
		return ticketsui.ServerLinkReadOnly
	}
	return ticketsui.ServerLinkUp
}

// WithServerConn sets the connection state the server indicator renders.
func (m Model) WithServerConn(c ServerConn) Model {
	m.serverConn = c
	return m
}
