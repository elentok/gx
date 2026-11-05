package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// showQueueIfAdded follows an add-to-queue toast with a switch to the Queue
// tab, so the new item is on screen. A total failure stays on the Tickets tab.
func showQueueIfAdded(added int, toast tea.Cmd) tea.Cmd {
	if added == 0 {
		return toast
	}
	return tea.Batch(toast, nav.Switch(nav.ViewState{Tab: nav.TabQueue}))
}

// WithServer connects the tab: rows come from the state the app shell
// delivers (WithServerState). Without it the tab reads the store itself
// (server_refresh.go).
func (m Model) WithServer(api tabapi.ServerAPI) Model {
	m.server.api = api
	return m
}

// WithSessions gives the tab the session sub-client; nil leaves "start session"
// and "investigate" unavailable.
func (m Model) WithSessions(s tabapi.SessionsAPI) Model {
	m.sessions = s
	return m
}

// WithAdmin gives the tab the admin sub-client; nil leaves the repair verbs
// answering "server not running".
func (m Model) WithAdmin(a tabapi.AdminAPI) Model {
	m.admin = a
	return m
}

// WithTickets gives the tab the tickets sub-client; nil leaves the menu's
// orchestration writes answering "server not running".
func (m Model) WithTickets(t tabapi.TicketsAPI) Model {
	m.tickets = t
	return m
}

// WithQueue gives the tab the queue sub-client.
func (m Model) WithQueue(q tabapi.QueueAPI) Model {
	m.queue = q
	return m
}

// WithReads gives the tab the reads sub-client.
func (m Model) WithReads(r tabapi.ReadsAPI) Model {
	m.reads = r
	return m
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet, so the tab keeps
// showing "loading…". While the tab reads the store itself (server down) the
// shell's state is stale and ignored; the crossing into down loads from disk.
func (m Model) WithServerState(st *viewmodel.State, link tabapi.ServerLink) (tea.Model, tea.Cmd) {
	apply, loadDisk := m.server.receive(st, link)
	if loadDisk {
		return m, cmdLoadDisk(m.scratchDir())
	}
	if !apply {
		return m, nil
	}
	firstLoad := !m.loaded
	m.loaded = true
	m = m.applyServerRows()
	if firstLoad {
		m.sidebar.SelectFirstEpic()
	}
	return m, nil
}

// WithCwdProject scopes the tab to the registered project the TUI started in;
// "" means the cwd is not a registered project, so the tab shows all.
func (m Model) WithCwdProject(name string) Model {
	m.server.scope.CwdProject = name
	return m
}

// serverBinding is the tab's link to the server: the client, the last
// delivered snapshot and the project scope applied to it.
type serverBinding struct {
	// api feeds vm; nil keeps the tab reading the store for good.
	api tabapi.ServerAPI
	// link is the link half of the last WithServerState delivery; nothing else
	// writes it.
	link tabapi.ServerLink
	// older: the last snapshot came from a server without rendered status; the
	// tab shows tabapi.OlderServerNotice and no tickets.
	older bool
	vm    viewmodel.State
	// scope is which projects the tab shows; owned by the tab, so no snapshot
	// or event can reset it.
	scope viewmodel.Scope
}

// receive takes a WithServerState delivery. apply reports that vm changed and
// the rows need rebuilding; loadDisk that the link just crossed into down, so
// the tab must load from the store. While the tab reads the store itself the
// shell's state is stale and ignored.
func (s *serverBinding) receive(st *viewmodel.State, link tabapi.ServerLink) (apply, loadDisk bool) {
	wasDown := s.link == tabapi.ServerLinkDown
	s.link = link
	if s.readsDisk() {
		s.older = false
		return false, link == tabapi.ServerLinkDown && !wasDown
	}
	if st == nil {
		return false, false
	}
	s.vm = *st
	s.older = st.OlderServer()
	if s.older {
		s.vm = viewmodel.State{}
	}
	return true, false
}

// toggleScope flips between the cwd project and all projects. ok is false when
// the cwd is no registered project; msg is the toast text either way.
func (s *serverBinding) toggleScope() (msg string, ok bool) {
	if s.scope.CwdProject == "" {
		return s.scope.UnregisteredHint(), false
	}
	s.scope = s.scope.Toggle()
	if s.scope.AllProjects {
		return "all projects", true
	}
	return "project: " + s.scope.CwdProject, true
}

// toggleProjectScope is "tp".
func (m Model) toggleProjectScope() (tea.Model, tea.Cmd) {
	msg, ok := m.server.toggleScope()
	if !ok {
		return m, notify.Info(msg)
	}
	return m.applyServerRows(), notify.Info(msg)
}

// updateServer handles the server-mode messages; ok is false for any other msg.
func updateServer(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case rowactions.EpicQueuedMsg:
		return showQueueIfAdded(msg.Queued, msg.Toast()), true

	case queue.ServerDoneMsg:
		if msg.Problem != "" {
			return notify.Error("refused: " + msg.Problem), true
		}
		return notify.Success(msg.OK), true

	case rowactions.ServerWriteMsg:
		return msg.Toast(), true
	}
	return nil, false
}

func (m Model) applyServerRows() Model {
	m.sidebar.Epics = tabapi.EpicsFromViewModel(m.server.vm, m.server.scope)
	m.clampSelected()
	return m
}
