package tickets

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tree"
	"github.com/elentok/gx/viewmodel"
)

// ServerAPI is the slice of apiclient.Client the tab uses in server mode.
type ServerAPI interface {
	Snapshot(ctx context.Context) (server.Snapshot, error)
	Events(ctx context.Context, since uint64) (<-chan server.Event, error)
	QueueItems(ctx context.Context) ([]server.QueueItem, error)
	QueueAdd(ctx context.Context, address, agent string) (server.QueueResult, error)
	QueueReplace(ctx context.Context, project string, items []server.QueueItem) (server.QueueResult, error)
	QueueRemove(ctx context.Context, address string) (server.QueueResult, error)
	QueuePause(ctx context.Context) (server.QueueResult, error)
	QueueResume(ctx context.Context) (server.QueueResult, error)
	Budget(ctx context.Context) (server.BudgetStatus, error)
	BudgetOverride(ctx context.Context) (server.BudgetResult, error)
	TicketPause(ctx context.Context, address, reason string) (server.QueueResult, error)
	TicketCancel(ctx context.Context, address string, stop bool) (server.QueueResult, error)
	TicketRelaunch(ctx context.Context, address string, stop bool) (server.QueueResult, error)
	TicketUnmute(ctx context.Context, address string) (server.QueueResult, error)
	TicketApprove(ctx context.Context, address string) (server.QueueResult, error)
	EpicMerge(ctx context.Context, address string) (server.QueueResult, error)
	EpicStashMerge(ctx context.Context, address string) (server.QueueResult, error)
	Detail(ctx context.Context, address string) (server.TicketDetail, error)
	Repair(ctx context.Context, verb string, req server.RepairRequest) (server.RepairResult, error)
	DesignWorktree(ctx context.Context, req server.DesignWorktreeRequest) (server.DesignWorktreeResult, error)
	Iterations(ctx context.Context) ([]server.IterationInfo, error)
	TicketChanged(ctx context.Context, address string) error
	FixAgent(ctx context.Context, req server.FixAgentRequest) (server.FixAgentResult, error)
	DefaultHarness(ctx context.Context) (server.DefaultHarnessStatus, error)
	SetDefaultHarness(ctx context.Context, req server.SetDefaultHarnessRequest) (server.SetDefaultHarnessResult, error)
}

// showQueueIfAdded follows an add-to-queue toast with a switch to the Queue
// tab, so the new item is on screen. A total failure stays on the Tickets tab.
func showQueueIfAdded(added int, toast tea.Cmd) tea.Cmd {
	if added == 0 {
		return toast
	}
	return tea.Batch(toast, nav.Switch(nav.ViewState{Tab: nav.TabQueue}))
}

// ResnapshotRequestedMsg asks the app shell, which owns the server stream, for
// a fresh snapshot (the tab's "R" and the reloads after a write).
type ResnapshotRequestedMsg struct{}

// WithServer connects the tab: rows come from the state the app shell
// delivers (WithServerState). Without it the tab reads the store itself
// (server_refresh.go).
func (m Model) WithServer(api ServerAPI) Model {
	m.serverAPI = api
	return m
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet, so the tab keeps
// showing "loading…". While the tab reads the store itself (server down) the
// shell's state is stale and ignored; the crossing into down loads from disk.
func (m Model) WithServerState(st *viewmodel.State, link ServerLink) (tea.Model, tea.Cmd) {
	wasDown := m.link == ServerLinkDown
	m.link = link
	if m.readsDisk() {
		m.olderServer = false
		if link == ServerLinkDown && !wasDown {
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
	case epicQueuedMsg:
		return m, showQueueIfAdded(msg.queued, msg.toast()), true

	case serverDoneMsg:
		if msg.problem != "" {
			return m, notify.Error("refused: " + msg.problem), true
		}
		return m, notify.Success(msg.ok), true

	case serverWriteMsg:
		return m, msg.toast(), true
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
	icon, _ := statusIconAndStyle(icons, ticketStatus(m.epicAt(r), t))
	width := triangleColumnWidth(icons) + 1 + lipgloss.Width(icon) + 1
	return strings.Repeat(" ", width) + blockedBySuffixStyle.Render(ellipsize(text, pauseReasonMaxRunes, icons.Ellipsis)), true
}

func (m Model) applyServerRows() Model {
	m.epics = epicsFromViewModel(m.vm, m.scope)
	m.clampSelected()
	return m
}

// epicsFromViewModel groups the view model's tickets (already in snapshot
// order) into the epics the sidebar renders. An epic is keyed by project and
// name, since two projects can share an epic name.
func epicsFromViewModel(vm viewmodel.State, scope viewmodel.Scope) []gxtickets.Epic {
	var epics []gxtickets.Epic
	byName := map[string]int{}
	for _, info := range vm.ScopedTickets(scope) {
		epic, id, ok := gxtickets.SplitTrailerValue(info.Address)
		if !ok {
			continue
		}
		project, _, _ := strings.Cut(info.Address, ":")
		name := project + ":" + epic
		i, seen := byName[name]
		if !seen {
			i = len(epics)
			byName[name] = i
			epics = append(epics, gxtickets.Epic{
				Name: name, Path: name, MergedAt: info.EpicMergedAt, IsMap: info.EpicIsMap,
				Status: info.EpicStatus, MergeTarget: info.EpicMergeTarget,
			})
		}
		epics[i].Tickets = append(epics[i].Tickets, ticketFromInfo(info, id))
	}
	return epics
}

func ticketFromInfo(info server.TicketInfo, id string) gxtickets.Ticket {
	// The number is the leading digits only: a multi-level fork id like
	// "27a1" ends in a digit, so trimming trailing letters would leave it
	// unparseable and every parent/blocker lookup for it would miss.
	digits := strings.IndexFunc(id, func(r rune) bool { return r < '0' || r > '9' })
	if digits < 0 {
		digits = len(id)
	}
	n, _ := strconv.Atoi(id[:digits])
	t := gxtickets.Ticket{
		Number:     n,
		Identifier: id,
		Title:      info.Title,
		Path:       info.Address,
		File:       info.File,
		Type:       info.Type,
		BlockedBy:  info.BlockedBy,
		Status:     info.Status,

		ExternalBlockers: info.ExternalBlockers,
		WidenedBlockers:  info.WidenedBlockers,

		ActualContextWindow:   info.ActualContextWindow,
		ExpectedContextWindow: info.ExpectedContextWindow,
		ElapsedTime:           info.ElapsedTime,
		ActualCost:            info.ActualCost,
		HadCodex:              info.HadCodex,
		Compactions:           info.Compactions,

		Category:   info.Category,
		ExportedAt: info.ExportedAt,
		Mutes:      info.Mutes,
	}
	if rendered, ok := gxtickets.ParseRenderedStatus(info.RenderedStatus); ok {
		t.Rendered = &rendered
	}
	if info.Parent != "" {
		_, parent, _ := gxtickets.SplitTrailerValue(info.Parent)
		t.Parent = &parent
	}
	return t
}
