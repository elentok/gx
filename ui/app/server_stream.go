package app

import (
	"context"
	"time"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	agentsui "github.com/elentok/gx/ui/agents"
	"github.com/elentok/gx/ui/agents/create"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// serverStatePage is a page that renders the shell's shared server state.
// st is nil until the first snapshot has arrived ("no snapshot yet"); link is
// how the shell currently reaches the server (a shell with no client is down).
// The command runs while the page is on screen (the Queue tab's spinner loop).
type serverStatePage interface {
	WithServerState(st *viewmodel.State, link tabapi.ServerLink) (tea.Model, tea.Cmd)
}

// serverStream is the shell's one subscription to the server: the shared state
// plus the bookkeeping that keeps exactly one event stream alive.
type serverStream struct {
	vm     viewmodel.State
	loaded bool // a snapshot has been applied
	// queue is vm's queue status, kept by setVM: working it out walks every
	// epic, too slow to redo on each render.
	queue   tabapi.QueueStatus
	queueOK bool

	// snapshotting coalesces re-snapshot requests; gen drops a snapshot reply
	// that a link-down (or a newer request) has outdated.
	snapshotting bool
	gen          uint64
	// snapshotFailing: the last snapshot failed, so further failures stay quiet
	// until one succeeds.
	snapshotFailing bool
	// queueRequested: R asked for the queue order along with the snapshot.
	queueRequested bool
	// resnapshotPending: an event asked for a re-snapshot and its debounce
	// tick is armed. resnapshotDue: the tick fired while a snapshot was in
	// flight, so another runs once that one lands.
	resnapshotPending bool
	resnapshotDue     bool

	// streamCtx is set from the moment a subscription is requested, so a
	// non-nil value means "a stream is live or being opened".
	streamCtx    context.Context
	streamStop   context.CancelFunc
	streamEvents <-chan wire.Event
}

// setVM replaces the shared state and its queue status.
func (s *serverStream) setVM(vm viewmodel.State) {
	s.vm = vm
	s.queue, s.queueOK = tabapi.QueueStatusOf(vm)
}

type streamSnapshotMsg struct {
	gen  uint64
	snap wire.Snapshot
	err  error
}

type streamSubscribedMsg struct {
	ctx    context.Context
	events <-chan wire.Event
	err    error
}

type streamEventMsg struct {
	ev     wire.Event
	events <-chan wire.Event
}

// streamEndedMsg: the stream closed. Whatever was missed is unknowable.
type streamEndedMsg struct {
	events <-chan wire.Event
}

type streamQueueMsg struct {
	items []wire.QueueItem
	err   error
}

// resnapshotTickMsg ends an event re-snapshot's debounce.
type resnapshotTickMsg struct{}

// resnapshotDebounce gathers a burst of events (an agent's ticket writes, a
// new session) into one snapshot: every open TUI re-snapshots on each event,
// and a snapshot is not cheap for the server.
const resnapshotDebounce = 100 * time.Millisecond

// streamWanted: the link is up (or read-only), no snapshot is in flight, and
// either no stream is live or being set up, or the last snapshot failed. The
// second case is the retry rule: a re-snapshot that fails while a stream is
// live is retried on the next probe tick.
func (l serverLink) streamWanted() bool {
	return l.deps != nil &&
		l.serverConn().State != ServerDown &&
		!l.stream.snapshotting &&
		(l.stream.streamCtx == nil || l.stream.snapshotFailing)
}

// startSnapshot requests one snapshot; requests made while one is in flight
// coalesce into it.
func (l serverLink) startSnapshot() (serverLink, tea.Cmd) {
	if l.stream.snapshotting || l.deps == nil {
		return l, nil
	}
	l.stream.snapshotting = true
	l.stream.gen++
	gen, client := l.stream.gen, l.deps.Client
	return l, func() tea.Msg {
		snap, err := client.Snapshot(context.Background())
		return streamSnapshotMsg{gen: gen, snap: snap, err: err}
	}
}

// cancelStream drops the live subscription; its pending messages are then
// ignored because they no longer match streamEvents.
func (l serverLink) cancelStream() serverLink {
	if l.stream.streamStop != nil {
		l.stream.streamStop()
	}
	l.stream.streamCtx, l.stream.streamStop, l.stream.streamEvents = nil, nil, nil
	return l
}

// onLinkChange reacts to a probe result: the store watch runs while down (the
// shell, not a page, watches it, so it runs whichever tab is on screen), the
// stream only while not. prev is the link before the result; the active page is
// told only when it changed, and hidden pages catch up when switched to.
func (l serverLink) onLinkChange(prev tabapi.ServerLink, env linkEnv) (serverLink, tea.Cmd, linkResult) {
	var cmds []tea.Cmd
	if l.serverConn().State == ServerDown {
		cmds = append(cmds, env.store.start(env.storeDir))
		l = l.cancelStream()
		l.stream.snapshotting = false
		l.stream.resnapshotPending, l.stream.resnapshotDue = false, false
		l.stream.gen++ // outdate an in-flight snapshot
	} else {
		env.store.halt()
		// Startup goes unknown → up without crossing the down line, so the
		// stream is started from "no stream yet", not from a transition.
		if l.streamWanted() {
			var snapshot tea.Cmd
			l, snapshot = l.startSnapshot()
			cmds = append(cmds, snapshot)
		}
	}
	return l, tea.Batch(cmds...), linkResult{deliver: l.serverConn().link() != prev}
}

// subscribe replaces the current stream with one from since; the older one is
// cancelled so it can never deliver again.
func (l serverLink) subscribe(since uint64) (serverLink, tea.Cmd) {
	l = l.cancelStream()
	ctx, stop := context.WithCancel(context.Background())
	l.stream.streamCtx, l.stream.streamStop = ctx, stop
	client := l.deps.Client
	return l, func() tea.Msg {
		ch, err := client.EventsAs(ctx, since, agentsui.FocusClient)
		return streamSubscribedMsg{ctx: ctx, events: ch, err: err}
	}
}

func cmdStreamNext(events <-chan wire.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return streamEndedMsg{events: events}
		}
		return streamEventMsg{ev: ev, events: events}
	}
}

func (l serverLink) cmdStreamQueue() tea.Cmd {
	queue := l.deps.Queue
	return func() tea.Msg {
		items, err := queue.Items(context.Background())
		return streamQueueMsg{items: items, err: err}
	}
}

func (l serverLink) streamEffects(e viewmodel.Effect) (serverLink, tea.Cmd) {
	var cmds []tea.Cmd
	if e&viewmodel.EffectResnapshot != 0 && !l.stream.resnapshotPending {
		l.stream.resnapshotPending = true
		cmds = append(cmds, tea.Tick(resnapshotDebounce, func(time.Time) tea.Msg { return resnapshotTickMsg{} }))
	}
	if e&viewmodel.EffectRefetchQueue != 0 {
		cmds = append(cmds, l.cmdStreamQueue())
	}
	return l, tea.Batch(cmds...)
}

// onSnapshotErr: the next probe tick retries while the link is still up; only
// the first failure of a streak toasts.
func (l serverLink) onSnapshotErr(err error) (serverLink, tea.Cmd) {
	if l.stream.snapshotFailing {
		return l, nil
	}
	l.stream.snapshotFailing = true
	return l, notify.Error("server snapshot: " + err.Error())
}

// onSnapshot applies a snapshot. It subscribes only when no stream is live,
// since the stream already carries what follows; a fresh subscription, an
// explicit R, or a first load is also when the queue order is fetched (otherwise
// only queue-changed refetches it).
func (l serverLink) onSnapshot(snap wire.Snapshot, env linkEnv) (serverLink, tea.Cmd) {
	l.stream.snapshotFailing = false
	firstLoad := !l.stream.loaded
	l.stream.setVM(l.stream.vm.ApplySnapshot(snap))
	l.stream.loaded = true

	var cmds []tea.Cmd
	fetchQueue := firstLoad || l.stream.queueRequested
	l.stream.queueRequested = false
	if l.stream.streamCtx == nil {
		var subscribe tea.Cmd
		l, subscribe = l.subscribe(snap.Seq)
		cmds = append(cmds, subscribe)
		fetchQueue = true
	}
	if fetchQueue {
		cmds = append(cmds, l.cmdStreamQueue())
	}
	if firstLoad {
		cmds = append(cmds, firstLoadHint(env.cwdProject()))
	}
	return l, tea.Batch(cmds...)
}

// firstLoadHint: reading the snapshot is not news, but not being in a
// registered project is worth saying once, whichever tab is open.
func firstLoadHint(cwdProject string) tea.Cmd {
	scope := viewmodel.Scope{CwdProject: cwdProject}
	if hint := scope.UnregisteredHint(); hint != "" {
		return notify.Info(hint)
	}
	return nil
}

// updateStream handles the stream's messages whichever tab is active;
// ok is false for any other msg.
func (l serverLink) updateStream(msg tea.Msg, env linkEnv) (serverLink, tea.Cmd, linkResult, bool) {
	switch msg := msg.(type) {
	case streamSnapshotMsg:
		if msg.gen != l.stream.gen {
			return l, nil, linkResult{}, true
		}
		l.stream.snapshotting = false
		due := l.stream.resnapshotDue
		l.stream.resnapshotDue = false
		if msg.err != nil {
			// The probe tick's retry covers whatever was due.
			l, cmd := l.onSnapshotErr(msg.err)
			return l, cmd, linkResult{}, true
		}
		l, cmd := l.onSnapshot(msg.snap, env)
		if due {
			var again tea.Cmd
			l, again = l.startSnapshot()
			cmd = tea.Batch(cmd, again)
		}
		return l, cmd, linkResult{deliver: true}, true

	case resnapshotTickMsg:
		l.stream.resnapshotPending = false
		switch {
		case l.serverConn().State == ServerDown:
		case l.stream.snapshotting:
			l.stream.resnapshotDue = true
		default:
			l, cmd := l.startSnapshot()
			return l, cmd, linkResult{}, true
		}
		return l, nil, linkResult{}, true

	case streamSubscribedMsg:
		if msg.ctx != l.stream.streamCtx {
			return l, nil, linkResult{}, true // a newer subscription replaced it
		}
		if msg.err != nil {
			l = l.cancelStream() // the next probe tick starts over
			return l, nil, linkResult{}, true
		}
		l.stream.streamEvents = msg.events
		return l, cmdStreamNext(msg.events), linkResult{}, true

	case streamEventMsg:
		if msg.events != l.stream.streamEvents {
			return l, nil, linkResult{}, true
		}
		seqBefore := l.stream.vm.Seq
		vm, effect := l.stream.vm.Reduce(msg.ev)
		l.stream.setVM(vm)
		var cmds []tea.Cmd
		if msg.ev.Seq > seqBefore+1 {
			// A gap: what was missed is unknowable, so the re-snapshot must
			// open a new subscription rather than keep this one.
			l = l.cancelStream()
		} else {
			cmds = append(cmds, cmdStreamNext(msg.events))
		}
		l, effects := l.streamEffects(effect)
		cmds = append(cmds, effects)
		res := linkResult{deliver: true}
		// Only an applied event toasts: a replayed duplicate or a gap is not news.
		if toast, ok := viewmodel.ToastFor(msg.ev); ok && l.stream.vm.Seq > seqBefore {
			cmds = append(cmds, toastCmd(toast))
		}
		if l.stream.vm.Seq > seqBefore && msg.ev.Type == wire.EventSessionFocus && msg.ev.Client == agentsui.FocusClient {
			res.focus, res.focusID = true, msg.ev.Address
		}
		return l, tea.Batch(cmds...), res, true

	case tabapi.ResnapshotRequestedMsg:
		// The queue is refetched when the snapshot lands. Down-mode "R" never
		// reaches here: the tabs reload from disk.
		if l.serverConn().State == ServerDown {
			return l, nil, linkResult{}, true
		}
		l.stream.queueRequested = true
		l, cmd := l.startSnapshot()
		return l, cmd, linkResult{}, true

	case streamEndedMsg:
		if msg.events != l.stream.streamEvents {
			return l, nil, linkResult{}, true
		}
		l = l.cancelStream()
		l, cmd := l.startSnapshot()
		return l, cmd, linkResult{}, true

	case streamQueueMsg:
		if msg.err != nil {
			return l, nil, linkResult{}, true
		}
		l.stream.setVM(l.stream.vm.SetQueue(msg.items))
		return l, nil, linkResult{deliver: true}, true
	}
	return l, nil, linkResult{}, false
}

func toastCmd(t viewmodel.Toast) tea.Cmd {
	switch t.Level {
	case wire.EventLevelSuccess:
		return notify.Success(t.Text)
	case wire.EventLevelError:
		return notify.Error(t.Text)
	}
	return notify.Warning(t.Text)
}

// serverState is the state to hand a page: nil before the first snapshot.
func (l serverLink) serverState() *viewmodel.State {
	if !l.stream.loaded {
		return nil
	}
	st := l.stream.vm
	return &st
}

// withServerState gives model the current state if it uses server state.
func (m Model) withServerState(model tea.Model) (tea.Model, tea.Cmd) {
	if p, ok := model.(serverStatePage); ok {
		return p.WithServerState(m.serverState(), m.serverConn().link())
	}
	return model, nil
}

// focusSession switches to the Agents tab with session id on screen, for a
// banner click or a Go to pane. The session is focused on the next state
// delivery, which is made here so it needn't wait for an event.
func (m Model) focusSession(id string) (Model, tea.Cmd) {
	m, cmd := m.switchTab(nav.ViewState{Tab: nav.TabAgents})
	current := m.activePage()
	agents, ok := current.model.(agentsui.Model)
	if !ok {
		return m, cmd
	}
	next, deliver := m.withServerState(agents.WithFocusSession(id))
	current.model = next
	m.setActivePage(current)
	return m, tea.Batch(cmd, deliver)
}

// startSession switches to the Agents tab with its create form open,
// pre-filled from a Tickets-tab "Start session…".
func (m Model) startSession(req tabapi.StartSessionMsg) (Model, tea.Cmd) {
	m, cmd := m.switchTab(nav.ViewState{Tab: nav.TabAgents})
	current := m.activePage()
	agents, ok := current.model.(agentsui.Model)
	if !ok {
		return m, cmd
	}
	agents, open := agents.OpenCreateForm(create.Prefill{Project: req.Project, Dir: req.Dir, Name: req.Name, Prompt: req.Prompt, Ticket: req.Ticket, RebaseNote: req.RebaseNote, MadeWorktree: req.MadeWorktree})
	current.model = agents
	m.setActivePage(current)
	return m, tea.Batch(cmd, open)
}

// deliverServerState hands the current state to the active page only; hidden
// pages catch up when they are switched to (see applySwitch).
func (m Model) deliverServerState() (Model, tea.Cmd) {
	current := m.activePage()
	if current.model == nil {
		return m, nil
	}
	var cmd tea.Cmd
	current.model, cmd = m.withServerState(current.model)
	m.setActivePage(current)
	return m, cmd
}
