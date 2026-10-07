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
	var addrs []string
	for _, epic := range m.epics {
		for _, t := range epic.Tickets {
			if m.checked[t.Path] && !epic.RenderedStatus(t).Terminal() {
				addrs = append(addrs, t.Path)
			}
		}
	}
	if len(addrs) == 0 {
		return m, notify.Info("check at least one ticket to enqueue")
	}
	agents := []string{string(ralphloop.AgentClaude), string(ralphloop.AgentCodex)}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:     fmt.Sprintf("Enqueue %d ticket(s)?", len(addrs)),
		Choices:    agents,
		Choice:     slices.Index(agents, string(serverEnqueueDefaultAgent)),
		ChoiceName: "Agent",
		AcceptWith: func(agent string) tea.Cmd { return m.cmdServerEnqueue(addrs, agent) },
	})
	return m, nil
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
		m.vm = m.vm.ApplySnapshot(msg.snap)
		m.loaded = true
		return m.applyServerRows(), tea.Batch(m.cmdServerSubscribe(msg.snap.Seq), m.cmdServerQueue()), true

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
	for _, info := range vm.Tickets {
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
		Type:       info.Type,
		BlockedBy:  info.BlockedBy,
		Status:     info.Status,
	}
	if info.Parent != "" {
		_, parent, _ := gxtickets.SplitTrailerValue(info.Parent)
		t.Parent = &parent
	}
	return t
}
