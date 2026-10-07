package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// serverProbeInterval spaces connection checks; a down daemon is polled, not spun on.
const serverProbeInterval = 2 * time.Second

// serverProbeTimeout bounds one handshake so a wedged server reads as down.
const serverProbeTimeout = time.Second

// ServerClient is what the shell needs from the API client: everything the
// tabs use, plus the handshake that tells up from down.
type ServerClient interface {
	ticketsui.ServerAPI
	Negotiate(ctx context.Context, build string) (apiclient.Negotiation, error)
}

// ServerDeps switches the shell to server mode; nil Settings.Server is the
// in-process mode.
type ServerDeps struct {
	Client ServerClient
	// Build is this client's build id, compared with the server's.
	Build string
	// Start launches the server (the Queue tab's "s" when the server is down).
	Start func(context.Context) error
}

// serverConnMsg is one probe's outcome.
type serverConnMsg struct{ conn ServerConn }

func (m Model) serverMode() bool { return m.settings.Server != nil }

func (m Model) cmdServerProbe() tea.Cmd {
	d := m.settings.Server
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), serverProbeTimeout)
		defer cancel()
		n, err := d.Client.Negotiate(ctx, d.Build)
		switch {
		case err != nil:
			return serverConnMsg{ServerConn{State: ServerDown}}
		case n.ReadOnly:
			return serverConnMsg{ServerConn{State: ServerReadOnly, PID: n.Pid}}
		}
		return serverConnMsg{ServerConn{State: ServerUp, PID: n.Pid}}
	}
}

func (m Model) cmdServerProbeLater() tea.Cmd {
	return tea.Tick(serverProbeInterval, func(time.Time) tea.Msg { return serverProbeTickMsg{} })
}

type serverProbeTickMsg struct{}

// updateServerConn handles the probe loop; ok is false for any other msg.
func (m Model) updateServerConn(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case serverProbeTickMsg:
		return m, m.cmdServerProbe(), true
	case serverConnMsg:
		prev := m.serverConn
		m.serverConn = msg.conn
		cmds := []tea.Cmd{m.cmdServerProbeLater()}
		// Pages start "up", so only a crossing of the down line is announced.
		wasDown := prev.State == ServerDown
		isDown := msg.conn.State == ServerDown
		var announce tea.Msg
		switch {
		case isDown && !wasDown:
			announce = ticketsui.ServerDownMsg{}
		case wasDown && !isDown:
			announce = ticketsui.ServerUpMsg{}
		}
		if announce != nil {
			cmds = append(cmds, m.broadcastToLivePages(announce))
		}
		if msg.conn.State == ServerReadOnly && prev.State != ServerReadOnly {
			m.markTicketsReadOnly()
		}
		return m, tea.Batch(cmds...), true
	}
	return m, nil, false
}

// markTicketsReadOnly covers the one link change that has no message: a
// version mismatch found after the Tickets tab was built.
func (m Model) markTicketsReadOnly() {
	p, ok := m.livePageByTab[nav.TabTickets]
	if !ok {
		return
	}
	if tm, ok := p.model.(ticketsui.Model); ok {
		p.model = tm.WithServerLink(ticketsui.ServerLinkReadOnly)
		m.livePageByTab[nav.TabTickets] = p
	}
}

// broadcastToLivePages delivers msg to every live tab, not just the active
// one: the Tickets and Queue tabs both track the connection.
func (m Model) broadcastToLivePages(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for tab, p := range m.livePageByTab {
		next, cmd := p.model.Update(msg)
		p.model = next
		m.livePageByTab[tab] = p
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}
