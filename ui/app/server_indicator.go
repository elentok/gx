package app

import (
	"image/color"

	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// ServerState is how this TUI sees the orchestrator server.
type ServerState int

const (
	// ServerUnknown: not probed yet, so no indicator is shown.
	ServerUnknown ServerState = iota
	ServerUp
	ServerDown
	// ServerReadOnly: API version mismatch; the client must not write.
	ServerReadOnly
	ServerHerdrUnavailable
)

// ServerConn is the connection state plus the server pid and web listener
// status, when known.
type ServerConn struct {
	State    ServerState
	PID      int
	WebURL   string
	WebError string
}

// serverBadge is how the title row draws one connection state: a colored icon
// and the words beside it, in a full and a short form for narrow rows.
type serverBadge struct {
	icon        string
	color       color.Color
	full, short string
}

// badge is the title row's server indicator; ok is false until the first probe.
func (c ServerConn) badge(icons ui.IconSet) (b serverBadge, ok bool) {
	switch c.State {
	case ServerUp:
		return serverBadge{icon: icons.ServerUp, color: ui.ColorGreen}, true
	case ServerDown:
		return serverBadge{icons.ServerDown, ui.ColorRed, "server down", "down"}, true
	case ServerReadOnly:
		return serverBadge{icons.ServerReadOnly, ui.ColorYellow, "read-only — restart", "read-only"}, true
	case ServerHerdrUnavailable:
		return serverBadge{icons.ServerHerdrUnavailable, ui.ColorYellow, "herdr unavailable", "no herdr"}, true
	}
	return serverBadge{}, false
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
	m.setServerConn(c)
	return m
}
