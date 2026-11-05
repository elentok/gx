package tickets

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
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
	m.serverAPI = api
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
	wasDown := m.link == tabapi.ServerLinkDown
	m.link = link
	if m.readsDisk() {
		m.olderServer = false
		if link == tabapi.ServerLinkDown && !wasDown {
			return m, m.cmdLoadDisk()
		}
		return m, nil
	}
	if st == nil {
		return m, nil
	}
	m.vm = *st
	firstLoad := !m.loaded
	m.loaded = true
	m.olderServer = st.OlderServer()
	if m.olderServer {
		m.vm = viewmodel.State{}
	}
	m = m.applyServerRows()
	if firstLoad {
		m.selectFirstEpic()
	}
	return m, nil
}

// WithCwdProject scopes the tab to the registered project the TUI started in;
// "" means the cwd is not a registered project, so the tab shows all.
func (m Model) WithCwdProject(name string) Model {
	m.scope.CwdProject = name
	return m
}

// toggleProjectScope is "tp": flip between the cwd project and all projects.
func (m Model) toggleProjectScope() (tea.Model, tea.Cmd) {
	if m.scope.CwdProject == "" {
		return m, notify.Info(m.scope.UnregisteredHint())
	}
	m.scope = m.scope.Toggle()
	label := "project: " + m.scope.CwdProject
	if m.scope.AllProjects {
		label = "all projects"
	}
	return m.applyServerRows(), notify.Info(label)
}

// updateServer handles the server-mode messages; ok is false for any other msg.
func (m Model) updateServer(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case rowactions.EpicQueuedMsg:
		return m, showQueueIfAdded(msg.Queued, msg.Toast()), true

	case queue.ServerDoneMsg:
		if msg.Problem != "" {
			return m, notify.Error("refused: " + msg.Problem), true
		}
		return m, notify.Success(msg.OK), true

	case rowactions.ServerWriteMsg:
		return m, msg.Toast(), true
	}
	return m, nil, false
}

// pendingSubtext is a queued ticket row's explain verdict, indented under the
// row's title. ok is false for any entry that isn't a queued ticket.
func (m Model) pendingSubtext(entry tree.Entry[sidebarNode]) (string, bool) {
	if entry.Value.kind != nodeTicket {
		return "", false
	}
	r, _ := rowFromEntry(entry)
	t := m.epicAt(r).Tickets[r.ticketIdx]
	p, ok := m.vm.PendingRowFor(t.Path)
	if !ok {
		return "", false
	}
	text := p.Verdict
	if p.Reason != "" {
		text += ": " + p.Reason
	}
	icons := m.icons()
	icon, _ := rowfmt.StatusIconAndStyle(icons, rowfmt.TicketStatus(m.epicAt(r), t))
	width := rowfmt.TriangleColumnWidth(icons) + 1 + lipgloss.Width(icon) + 1
	return strings.Repeat(" ", width) + rowfmt.BlockedBySuffixStyle.Render(rowfmt.Ellipsize(text, rowfmt.PauseReasonMaxRunes, icons.Ellipsis)), true
}

func (m Model) applyServerRows() Model {
	m.epics = tabapi.EpicsFromViewModel(m.vm, m.scope)
	m.clampSelected()
	return m
}
