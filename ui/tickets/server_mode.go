package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/keys"
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
// delivers (WithServerState). Without it the tab reads the store itself.
func (m Model) WithServer(api tabapi.ServerAPI) Model {
	m.server.API = api
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
	apply, loadDisk := m.server.Receive(st, link)
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
	m.server.Scope.CwdProject = name
	return m
}

// toggleProjectScope is "tp".
func (m Model) toggleProjectScope() (tea.Model, tea.Cmd) {
	msg, ok := m.server.ToggleScope()
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
	m.sidebar.Epics = m.server.Epics()
	m.clampSelected()
	return m
}

// serverBlockedBindings lists every binding that needs a writable server. A
// new server-backed binding needs an entry here — nothing else enforces that.
var serverBlockedBindings = map[keys.BindingID]bool{
	bindingTicketsFinishAndPause: true,
}

// keyGuard is the shared no-op check for server-backed keys: blocked
// reports whether the caller should return cmd (a toast with the reason).
func (m Model) keyGuard(id keys.BindingID) (cmd tea.Cmd, blocked bool) {
	if !serverBlockedBindings[id] {
		return nil, false
	}
	return m.server.DisabledToast()
}

// handleServerFinishAndPauseKey is "D" in server mode, an alias of Finish and pause kept
// for one release: stop the server handing out new work; running tickets finish.
func (m Model) handleServerFinishAndPauseKey() (tea.Model, tea.Cmd) {
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    queue.FinishAndPausePrompt,
		AcceptCmd: queue.CmdServerQueueMode("queue paused", m.queue.Pause),
	})
	return m, nil
}

// The Tickets tab's rows come from the server's event stream while it is
// reachable. While it is down — or the tab has no client at all — the tab reads
// the store, reloading when the app shell (which runs the store watch) says the
// store changed and when the tab is activated.

// OnPageActivated catches the tab up with the store: hidden tabs are not told
// about changes made while the server was down.
func (m Model) OnPageActivated() tea.Cmd {
	if !m.server.ReadsDisk() {
		return nil
	}
	return cmdLoadDisk(m.scratchDir())
}

// cmdLoadDisk reads `.scratch/` directly: the down-mode loader. A missing
// directory reads as zero epics, and an unreadable `.archive` as nothing
// archived.
func cmdLoadDisk(scratchDir string) tea.Cmd {
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		archivedEpicCount, _ := tickets.CountArchivedEpics(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err, archivedEpicCount: archivedEpicCount}
	}
}
