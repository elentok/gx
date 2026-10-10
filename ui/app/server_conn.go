package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// serverProbeInterval spaces connection checks; a down daemon is polled, not spun on.
const serverProbeInterval = 2 * time.Second

// serverProbeTimeout bounds one handshake. A server that is merely slow (a
// loaded machine) keeps its last state; only a refused or missing socket reads
// as down.
const serverProbeTimeout = 5 * time.Second

// ServerClient is what the shell needs from the API client: everything the
// tabs use, plus the handshake that tells up from down.
type ServerClient interface {
	ticketsui.ServerAPI
	Negotiate(ctx context.Context, build string) (apiclient.Negotiation, error)
}

// ServerDeps is how the shell reaches the server. A nil Settings.Server (no
// client could be built) leaves the shell permanently down.
type ServerDeps struct {
	Client ServerClient
	// Build is this client's build id, compared with the server's.
	Build string
	// Start launches the server (the Queue tab's "s" when the server is down).
	Start func(context.Context) error
}

// serverConnMsg is one handshake's outcome, to be applied to the shell. A
// manual probe (a page's "R", a finished start) sends it as is.
type serverConnMsg struct {
	conn ServerConn
	// slow: the handshake failed for a reason other than "nothing listens", so
	// the last known state stands.
	slow bool
}

// serverProbeResultMsg is the probe loop's outcome: applied like a serverConnMsg,
// and it arms the next tick, which a manual probe must not.
type serverProbeResultMsg struct{ serverConnMsg }

func (m Model) cmdServerProbe() tea.Cmd {
	d := m.settings.Server
	return func() tea.Msg { return serverProbeResultMsg{handshake(d)} }
}

func (m Model) cmdManualProbe() tea.Cmd {
	d := m.settings.Server
	return func() tea.Msg { return handshake(d) }
}

func handshake(d *ServerDeps) serverConnMsg {
	ctx, cancel := context.WithTimeout(context.Background(), serverProbeTimeout)
	defer cancel()
	n, err := d.Client.Negotiate(ctx, d.Build)
	switch {
	case err != nil && !apiclient.IsNotRunning(err):
		return serverConnMsg{slow: true}
	case err != nil:
		return serverConnMsg{conn: ServerConn{State: ServerDown}}
	case n.ReadOnly:
		return serverConnMsg{conn: ServerConn{State: ServerReadOnly, PID: n.Pid}}
	case n.HerdrUnavailable:
		return serverConnMsg{conn: ServerConn{State: ServerHerdrUnavailable, PID: n.Pid}}
	}
	return serverConnMsg{conn: ServerConn{State: ServerUp, PID: n.Pid}}
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
	case serverProbeResultMsg:
		m, cmd := m.applyServerConn(msg.serverConnMsg)
		return m, tea.Batch(m.cmdServerProbeLater(), cmd), true

	case serverConnMsg:
		m, cmd := m.applyServerConn(msg)
		return m, cmd, true

	case ticketsui.ProbeRequestedMsg:
		if m.settings.Server == nil {
			return m, nil, true // permanently down: nothing to ask
		}
		return m, m.cmdManualProbe(), true
	}
	return m, nil, false
}

// applyServerConn takes a handshake outcome; a slow one leaves the last state.
func (m Model) applyServerConn(msg serverConnMsg) (Model, tea.Cmd) {
	if msg.slow {
		return m, nil
	}
	prev := m.serverConn.link()
	m.serverConn = msg.conn
	return m.onLinkChange(prev)
}
