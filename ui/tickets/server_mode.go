package tickets

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tree"
	"github.com/elentok/gx/viewmodel"
)

// serverReconnectDelay spaces re-snapshot attempts while the server is
// unreachable, so a down daemon is polled rather than spun on.
const serverReconnectDelay = 2 * time.Second

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
	QueueDrain(ctx context.Context) (server.QueueResult, error)
	Budget(ctx context.Context) (server.BudgetStatus, error)
	BudgetOverride(ctx context.Context) (server.BudgetResult, error)
	TicketPark(ctx context.Context, address, reason string) (server.QueueResult, error)
	TicketCancel(ctx context.Context, address string, stop bool) (server.QueueResult, error)
	TicketRelaunch(ctx context.Context, address string) (server.QueueResult, error)
	Repair(ctx context.Context, verb string, req server.RepairRequest) (server.RepairResult, error)
	Iterations(ctx context.Context) ([]server.IterationInfo, error)
	TicketChanged(ctx context.Context, address string) error
}

// serverEnqueueDefaultAgent is preselected in the "a" confirm; it matches the
// server's own default for an empty agent.
const serverEnqueueDefaultAgent = ralphloop.AgentClaude

// serverEnqueuedMsg reports a finished "a" enqueue: how many addresses the
// server accepted and the first failure or refusal, if any.
type serverEnqueuedMsg struct {
	added   int
	problem string
}

// handleServerEnqueueKey is "a" in server mode: enqueue every checked,
// not-yet-terminal ticket, live run or not, with the agent picked in the
// confirm.
func (m Model) handleServerEnqueueKey() (tea.Model, tea.Cmd) {
	addrs := m.checkedPendingAddresses()
	if len(addrs) == 0 {
		return m, notify.Info("check at least one ticket to enqueue")
	}
	m.confirm = m.openAgentConfirm(fmt.Sprintf("Enqueue %d ticket(s)?", len(addrs)),
		func(agent string) tea.Cmd { return m.cmdServerEnqueue(addrs, agent) })
	return m, nil
}

// checkedPendingAddresses is every checked, not-yet-terminal ticket.
func (m Model) checkedPendingAddresses() []string {
	var addrs []string
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if m.checked[t.Path] && !epic.RenderedStatus(t).Terminal() {
				addrs = append(addrs, t.Path)
			}
		}
	}
	return addrs
}

func (m Model) openAgentConfirm(prompt string, accept func(agent string) tea.Cmd) confirm.Model {
	agents := []string{string(ralphloop.AgentClaude), string(ralphloop.AgentCodex)}
	return m.confirm.Open(confirm.Options{
		Prompt:     prompt,
		Choices:    agents,
		Choice:     slices.Index(agents, string(serverEnqueueDefaultAgent)),
		ChoiceName: "Agent",
		AcceptWith: accept,
	})
}

// serverReplacedMsg reports a finished "r": the server's refusal or error, if any.
type serverReplacedMsg struct {
	count   int
	problem string
}

// handleServerReplaceKey is "r" in server mode: replace one project's pending
// entries with the checked tickets, using the agent picked in the confirm. The
// server owns the live-run guard and refuses with ticket-live.
func (m Model) handleServerReplaceKey() (tea.Model, tea.Cmd) {
	addrs := m.checkedPendingAddresses()
	if len(addrs) == 0 {
		return m, notify.Info("check at least one ticket to replace the queue")
	}
	project, _, _ := strings.Cut(addrs[0], ":")
	for _, a := range addrs {
		if p, _, _ := strings.Cut(a, ":"); p != project {
			return m, notify.Info("replace works on one project; checked tickets span several")
		}
	}
	m.confirm = m.openAgentConfirm(fmt.Sprintf("Replace %s's queue with %d ticket(s)?", project, len(addrs)),
		func(agent string) tea.Cmd { return m.cmdServerReplace(project, addrs, agent) })
	return m, nil
}

func (m Model) cmdServerReplace(project string, addrs []string, agent string) tea.Cmd {
	api := m.serverAPI
	items := make([]server.QueueItem, len(addrs))
	for i, a := range addrs {
		items[i] = server.QueueItem{Address: a, Agent: agent}
	}
	return func() tea.Msg {
		res, err := api.QueueReplace(context.Background(), project, items)
		switch {
		case err != nil:
			return serverReplacedMsg{problem: err.Error()}
		case res.Refused:
			return serverReplacedMsg{problem: res.Message}
		}
		return serverReplacedMsg{count: len(items)}
	}
}

func (m Model) cmdServerEnqueue(addrs []string, agent string) tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		out := serverEnqueuedMsg{}
		for _, addr := range addrs {
			res, err := api.QueueAdd(context.Background(), addr, agent)
			switch {
			case err != nil:
				out.problem = addr + ": " + err.Error()
			case res.Refused:
				out.problem = addr + ": " + res.Message
			default:
				out.added++
				continue
			}
			break
		}
		return out
	}
}

type serverSnapshotMsg struct {
	snap server.Snapshot
	err  error
}

type serverSubscribedMsg struct {
	events <-chan server.Event
	err    error
}

type serverEventMsg struct {
	ev     server.Event
	events <-chan server.Event
}

// serverStreamEndedMsg: the stream closed (server dropped us, restarted, or
// went away). Whatever we missed is unknowable, so re-snapshot.
type serverStreamEndedMsg struct{}

type serverRetryMsg struct{}

type serverQueueMsg struct {
	items []server.QueueItem
	err   error
}

// WithServer switches the tab to server mode: rows come from the view model
// fed by the server's snapshot and event stream, never from `.scratch/`.
func (m Model) WithServer(api ServerAPI) Model {
	m.serverAPI = api
	// The disk poll is the non-server data source; it must never start.
	m.autoRefreshStarted = true
	return m
}

// WithCwdProject scopes the tab to the registered project the TUI started in;
// "" means the cwd is not a registered project, so the tab shows all.
func (m Model) WithCwdProject(name string) Model {
	m.vm.CwdProject = name
	m.scopeKnown = true
	return m
}

// toggleProjectScope is "tp": flip between the cwd project and all projects.
func (m Model) toggleProjectScope() (tea.Model, tea.Cmd) {
	if m.vm.CwdProject == "" {
		return m, notify.Info(m.vm.UnregisteredHint())
	}
	m.vm = m.vm.ToggleAllProjects()
	label := "project: " + m.vm.CwdProject
	if m.vm.AllProjects {
		label = "all projects"
	}
	return m.applyServerRows(), notify.Info(label)
}

func (m Model) serverMode() bool { return m.serverAPI != nil }

func (m Model) cmdServerSnapshot() tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		snap, err := api.Snapshot(context.Background())
		return serverSnapshotMsg{snap: snap, err: err}
	}
}

func (m Model) cmdServerSubscribe(since uint64) tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		ch, err := api.Events(context.Background(), since)
		return serverSubscribedMsg{events: ch, err: err}
	}
}

func cmdServerNextEvent(events <-chan server.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return serverStreamEndedMsg{}
		}
		return serverEventMsg{ev: ev, events: events}
	}
}

func (m Model) cmdServerQueue() tea.Cmd {
	api := m.serverAPI
	return func() tea.Msg {
		items, err := api.QueueItems(context.Background())
		return serverQueueMsg{items: items, err: err}
	}
}

func (m Model) cmdServerResnapshotLater() tea.Cmd {
	return tea.Tick(serverReconnectDelay, func(time.Time) tea.Msg { return serverRetryMsg{} })
}

func (m Model) cmdServerEffects(e viewmodel.Effect) tea.Cmd {
	var cmds []tea.Cmd
	if e&viewmodel.EffectResnapshot != 0 {
		cmds = append(cmds, m.cmdServerSnapshot())
	}
	if e&viewmodel.EffectRefetchQueue != 0 {
		cmds = append(cmds, m.cmdServerQueue())
	}
	return tea.Batch(cmds...)
}

// updateServer handles the server-mode messages; ok is false for any other msg.
func (m Model) updateServer(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case serverSnapshotMsg:
		if msg.err != nil {
			return m, tea.Batch(notify.Error("server snapshot: "+msg.err.Error()), m.cmdServerResnapshotLater()), true
		}
		firstLoad := !m.loaded
		m.vm = m.vm.ApplySnapshot(msg.snap)
		m.loaded = true
		cmds := []tea.Cmd{m.cmdServerSubscribe(msg.snap.Seq), m.cmdServerQueue()}
		if hint := m.vm.UnregisteredHint(); hint != "" && firstLoad && m.scopeKnown {
			cmds = append(cmds, notify.Info(hint))
		}
		return m.applyServerRows(), tea.Batch(cmds...), true

	case serverSubscribedMsg:
		if msg.err != nil {
			return m, m.cmdServerResnapshotLater(), true
		}
		return m, cmdServerNextEvent(msg.events), true

	case serverEventMsg:
		var effect viewmodel.Effect
		seqBefore := m.vm.Seq
		m.vm, effect = m.vm.Reduce(msg.ev)
		cmds := []tea.Cmd{cmdServerNextEvent(msg.events), m.cmdServerEffects(effect)}
		// Only an applied event toasts: a replayed duplicate or a gap is not news.
		if text, ok := viewmodel.ToastFor(msg.ev); ok && m.vm.Seq > seqBefore {
			cmds = append(cmds, notify.Warning(text))
		}
		return m.applyServerRows(), tea.Batch(cmds...), true

	case serverRetryMsg:
		return m, m.cmdServerSnapshot(), true

	case serverStreamEndedMsg:
		return m, m.cmdServerSnapshot(), true

	case serverEnqueuedMsg:
		note := notify.Success(fmt.Sprintf("enqueued %d ticket(s)", msg.added))
		if msg.problem != "" {
			note = notify.Error(fmt.Sprintf("enqueued %d, stopped at %s", msg.added, msg.problem))
		}
		return m, tea.Batch(note, m.cmdServerQueue()), true

	case serverReplacedMsg:
		if msg.problem != "" {
			return m, notify.Error("replace refused: " + msg.problem), true
		}
		return m, tea.Batch(notify.Success(fmt.Sprintf("queue replaced with %d ticket(s)", msg.count)), m.cmdServerQueue()), true

	case serverDoneMsg:
		if msg.problem != "" {
			return m, notify.Error("refused: " + msg.problem), true
		}
		return m, tea.Batch(notify.Success(msg.ok), m.cmdServerQueue()), true

	case serverWriteMsg:
		return m, msg.toast(), true

	case serverQueueMsg:
		if msg.err == nil {
			m.vm = m.vm.SetQueue(msg.items)
		}
		return m, nil, true
	}
	return m, nil, false
}

// notificationsForRun is the chat config a TUI-started run wires sinks from.
// In server mode it is empty: the server sends chat once, and a TUI sink
// would duplicate it.
func (m Model) notificationsForRun() config.NotificationsConfig {
	if m.serverMode() {
		return config.NotificationsConfig{}
	}
	return m.settings.Notifications
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
	icon, _ := statusIconAndStyle(icons, m.epicAt(r).RenderedStatus(t))
	width := triangleColumnWidth(icons) + 1 + lipgloss.Width(icons.CheckboxUnchecked) + 1 + lipgloss.Width(icon) + 1
	return strings.Repeat(" ", width) + blockedBySuffixStyle.Render(ellipsize(text, parkReasonMaxRunes, icons.Ellipsis)), true
}

func (m Model) applyServerRows() Model {
	m.epics = epicsFromViewModel(m.vm)
	m.clampSelected()
	return m
}

// epicsFromViewModel groups the view model's tickets (already in snapshot
// order) into the epics the sidebar renders. An epic is keyed by project and
// name, since two projects can share an epic name.
func epicsFromViewModel(vm viewmodel.State) []gxtickets.Epic {
	var epics []gxtickets.Epic
	byName := map[string]int{}
	for _, info := range vm.ScopedTickets() {
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
			epics = append(epics, gxtickets.Epic{Name: name, Path: name})
		}
		epics[i].Tickets = append(epics[i].Tickets, ticketFromInfo(info, id))
	}
	return epics
}

func ticketFromInfo(info server.TicketInfo, id string) gxtickets.Ticket {
	n, _ := strconv.Atoi(strings.TrimRight(id, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	t := gxtickets.Ticket{
		Number:     n,
		Identifier: id,
		Title:      info.Title,
		Path:       info.Address,
		File:       info.File,
		Type:       info.Type,
		BlockedBy:  info.BlockedBy,
		Status:     info.Status,

		ActualContextWindow:   info.ActualContextWindow,
		ExpectedContextWindow: info.ExpectedContextWindow,
		ElapsedTime:           info.ElapsedTime,
		ActualCost:            info.ActualCost,
		Compactions:           info.Compactions,
	}
	if info.Parent != "" {
		_, parent, _ := gxtickets.SplitTrailerValue(info.Parent)
		t.Parent = &parent
	}
	return t
}
