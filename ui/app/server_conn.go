package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
	agentsui "github.com/elentok/gx/ui/agents"
	"github.com/elentok/gx/ui/tickets/tabapi"
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
	tabapi.ServerAPI
	agentsui.SessionClient
	Negotiate(ctx context.Context, build string) (apiclient.Negotiation, error)
	// EventsAs subscribes under the Agents tab's focus client name, so its
	// focus reports count while the stream is open.
	EventsAs(ctx context.Context, since uint64, client string) (<-chan wire.Event, error)
}

// ServerSessions is the session sub-client: what the Agents, Tickets and
// Queue tabs each use of it, together.
type ServerSessions interface {
	agentsui.Sessions
	tabapi.SessionsAPI
}

// ServerDeps is how the shell reaches the server. A nil Settings.Server (no
// client could be built) leaves the shell permanently down.
type ServerDeps struct {
	Client   ServerClient
	Sessions ServerSessions
	Admin    tabapi.AdminAPI
	Tickets  tabapi.TicketsAPI
	Queue    tabapi.QueueAPI
	Reads    tabapi.ReadsAPI
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

func (l serverLink) cmdServerProbe() tea.Cmd {
	d := l.deps
	return func() tea.Msg { return serverProbeResultMsg{handshake(d)} }
}

func (l serverLink) cmdManualProbe() tea.Cmd {
	d := l.deps
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
	}
	conn := ServerConn{State: ServerUp, PID: n.Pid, WebURL: n.WebURL, WebError: n.WebError, WebSetupPending: n.WebSetupPending}
	switch {
	case n.ReadOnly:
		conn.State = ServerReadOnly
	case n.HerdrUnavailable:
		conn.State = ServerHerdrUnavailable
	}
	return serverConnMsg{conn: conn}
}

func cmdServerProbeLater() tea.Cmd {
	return tea.Tick(serverProbeInterval, func(time.Time) tea.Msg { return serverProbeTickMsg{} })
}

type serverProbeTickMsg struct{}

// updateConn handles the probe loop; ok is false for any other msg.
func (l serverLink) updateConn(msg tea.Msg, env linkEnv) (serverLink, tea.Cmd, linkResult, bool) {
	switch msg := msg.(type) {
	case serverProbeTickMsg:
		return l, l.cmdServerProbe(), linkResult{}, true
	case serverProbeResultMsg:
		l, cmd, res := l.applyServerConn(msg.serverConnMsg, env)
		return l, tea.Batch(cmdServerProbeLater(), cmd), res, true

	case serverConnMsg:
		l, cmd, res := l.applyServerConn(msg, env)
		return l, cmd, res, true

	case tabapi.ProbeRequestedMsg:
		if l.deps == nil {
			return l, nil, linkResult{}, true // permanently down: nothing to ask
		}
		return l, l.cmdManualProbe(), linkResult{}, true
	}
	return l, nil, linkResult{}, false
}

// applyServerConn takes a handshake outcome; a slow one leaves the last state.
func (l serverLink) applyServerConn(msg serverConnMsg, env linkEnv) (serverLink, tea.Cmd, linkResult) {
	if msg.slow {
		return l, nil, linkResult{}
	}
	prev := l.serverConn().link()
	l.setServerConn(msg.conn)
	return l.onLinkChange(prev, env)
}
