package app

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

// serverLink is the shell's connection to the server: the handshake state and
// the one event-stream subscription. It never touches the page stack; what it
// needs from the pages it reports in a linkResult.
type serverLink struct {
	// deps is nil when no client could be built: the link is then permanently down.
	deps *ServerDeps
	// conn stays a pointer on purpose: Model is copied on every message and
	// the About provider reads the cell too; go through serverConn and
	// setServerConn.
	conn   *atomic.Pointer[ServerConn]
	stream serverStream
}

func newServerLink(deps *ServerDeps) serverLink {
	return serverLink{deps: deps, conn: &atomic.Pointer[ServerConn]{}}
}

// linkEnv is what the link borrows from the shell for one message.
type linkEnv struct {
	// store is the shell's down-mode store watch, started and halted by link changes.
	store    *storeWatch
	storeDir string
	// cwdProject is looked up only when the first snapshot lands.
	cwdProject func() string
}

// linkResult is what the shell must do to its pages after a link update.
type linkResult struct {
	// deliver: hand the new server state to the active page.
	deliver bool
	// focus: switch to the Agents tab with session focusID on screen.
	focus   bool
	focusID string
}

// Update handles the probe loop and the event stream; handled is false for any
// other msg.
func (l serverLink) Update(msg tea.Msg, env linkEnv) (next serverLink, cmd tea.Cmd, res linkResult, handled bool) {
	if next, cmd, res, handled = l.updateConn(msg, env); handled {
		return next, cmd, res, true
	}
	return l.updateStream(msg, env)
}

// linkEnv gathers the link's inputs from the shell. It points at m's own
// store watch, so m must be the copy the caller goes on to return.
func (m *Model) linkEnv() linkEnv {
	settings := m.settings
	return linkEnv{
		store:      &m.store,
		storeDir:   m.storeDir(),
		cwdProject: func() string { return cwdProjectName(settings.TicketStore, settings.ActiveWorktreePath) },
	}
}

// updateServerLink routes the link's messages and applies its result to the
// pages.
func (m Model) updateServerLink(msg tea.Msg) (Model, tea.Cmd, bool) {
	env := m.linkEnv()
	link, cmd, res, ok := m.serverLink.Update(msg, env)
	if !ok {
		return m, nil, false
	}
	m.serverLink = link
	cmds := []tea.Cmd{cmd}
	if res.focus {
		var focus tea.Cmd
		m, focus = m.focusSession(res.focusID)
		cmds = append(cmds, focus)
	}
	if res.deliver {
		var deliver tea.Cmd
		m, deliver = m.deliverServerState()
		cmds = append(cmds, deliver)
	}
	return m, tea.Batch(cmds...), true
}
