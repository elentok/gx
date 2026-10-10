package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/viewmodel"
)

// serverStatePage is a page that renders the shell's shared server state.
// st is nil until the first snapshot has arrived ("no snapshot yet").
type serverStatePage interface {
	WithServerState(st *viewmodel.State) tea.Model
}

// serverStream is the shell's one subscription to the server: the shared state
// plus the bookkeeping that keeps exactly one event stream alive.
type serverStream struct {
	vm     viewmodel.State
	loaded bool // a snapshot has been applied

	// snapshotting coalesces re-snapshot requests; gen drops a snapshot reply
	// that a link-down (or a newer request) has outdated.
	snapshotting bool
	gen          uint64

	// streamCtx is set from the moment a subscription is requested, so a
	// non-nil value means "a stream is live or being opened".
	streamCtx    context.Context
	streamStop   context.CancelFunc
	streamEvents <-chan server.Event
}

type streamSnapshotMsg struct {
	gen  uint64
	snap server.Snapshot
	err  error
}

type streamSubscribedMsg struct {
	ctx    context.Context
	events <-chan server.Event
	err    error
}

type streamEventMsg struct {
	ev     server.Event
	events <-chan server.Event
}

// streamEndedMsg: the stream closed. Whatever was missed is unknowable.
type streamEndedMsg struct {
	events <-chan server.Event
}

type streamQueueMsg struct {
	items []server.QueueItem
	err   error
}

// streamWanted: the link is up (or read-only) but no stream is live or being
// set up, so a snapshot must start one. This is also the retry rule after a
// failed snapshot: the next probe tick finds the shell still stream-less.
func (m Model) streamWanted() bool {
	return m.settings.Server != nil &&
		m.serverConn.State != ServerDown &&
		m.stream.streamCtx == nil && !m.stream.snapshotting
}

// startSnapshot requests one snapshot; requests made while one is in flight
// coalesce into it.
func (m Model) startSnapshot() (Model, tea.Cmd) {
	if m.stream.snapshotting || m.settings.Server == nil {
		return m, nil
	}
	m.stream.snapshotting = true
	m.stream.gen++
	gen, client := m.stream.gen, m.settings.Server.Client
	return m, func() tea.Msg {
		snap, err := client.Snapshot(context.Background())
		return streamSnapshotMsg{gen: gen, snap: snap, err: err}
	}
}

// cancelStream drops the live subscription; its pending messages are then
// ignored because they no longer match streamEvents.
func (m Model) cancelStream() Model {
	if m.stream.streamStop != nil {
		m.stream.streamStop()
	}
	m.stream.streamCtx, m.stream.streamStop, m.stream.streamEvents = nil, nil, nil
	return m
}

// onLinkChange starts or stops the stream after a probe result.
func (m Model) onLinkChange() (Model, tea.Cmd) {
	if m.serverConn.State == ServerDown {
		m = m.cancelStream()
		m.stream.snapshotting = false
		m.stream.gen++ // outdate an in-flight snapshot
		return m, nil
	}
	if m.streamWanted() {
		return m.startSnapshot()
	}
	return m, nil
}

// subscribe replaces the current stream with one from since; the older one is
// cancelled so it can never deliver again.
func (m Model) subscribe(since uint64) (Model, tea.Cmd) {
	m = m.cancelStream()
	ctx, stop := context.WithCancel(context.Background())
	m.stream.streamCtx, m.stream.streamStop = ctx, stop
	client := m.settings.Server.Client
	return m, func() tea.Msg {
		ch, err := client.Events(ctx, since)
		return streamSubscribedMsg{ctx: ctx, events: ch, err: err}
	}
}

func cmdStreamNext(events <-chan server.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return streamEndedMsg{events: events}
		}
		return streamEventMsg{ev: ev, events: events}
	}
}

func (m Model) cmdStreamQueue() tea.Cmd {
	client := m.settings.Server.Client
	return func() tea.Msg {
		items, err := client.QueueItems(context.Background())
		return streamQueueMsg{items: items, err: err}
	}
}

func (m Model) streamEffects(e viewmodel.Effect) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	if e&viewmodel.EffectResnapshot != 0 {
		var cmd tea.Cmd
		m, cmd = m.startSnapshot()
		cmds = append(cmds, cmd)
	}
	if e&viewmodel.EffectRefetchQueue != 0 {
		cmds = append(cmds, m.cmdStreamQueue())
	}
	return m, tea.Batch(cmds...)
}

// updateServerStream handles the stream's messages whichever tab is active;
// ok is false for any other msg.
func (m Model) updateServerStream(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case streamSnapshotMsg:
		if msg.gen != m.stream.gen {
			return m, nil, true
		}
		m.stream.snapshotting = false
		if msg.err != nil {
			// The next probe tick retries while the link is still up.
			return m, nil, true
		}
		m.stream.vm = m.stream.vm.ApplySnapshot(msg.snap)
		m.stream.loaded = true
		m, subscribe := m.subscribe(msg.snap.Seq)
		return m.deliverServerState(), tea.Batch(subscribe, m.cmdStreamQueue()), true

	case streamSubscribedMsg:
		if msg.ctx != m.stream.streamCtx {
			return m, nil, true // a newer subscription replaced it
		}
		if msg.err != nil {
			m = m.cancelStream() // the next probe tick starts over
			return m, nil, true
		}
		m.stream.streamEvents = msg.events
		return m, cmdStreamNext(msg.events), true

	case streamEventMsg:
		if msg.events != m.stream.streamEvents {
			return m, nil, true
		}
		var effect viewmodel.Effect
		m.stream.vm, effect = m.stream.vm.Reduce(msg.ev)
		m, effects := m.streamEffects(effect)
		return m.deliverServerState(), tea.Batch(cmdStreamNext(msg.events), effects), true

	case streamEndedMsg:
		if msg.events != m.stream.streamEvents {
			return m, nil, true
		}
		m = m.cancelStream()
		m, cmd := m.startSnapshot()
		return m, cmd, true

	case streamQueueMsg:
		if msg.err != nil {
			return m, nil, true
		}
		m.stream.vm = m.stream.vm.SetQueue(msg.items)
		return m.deliverServerState(), nil, true
	}
	return m, nil, false
}

// serverState is the state to hand a page: nil before the first snapshot.
func (m Model) serverState() *viewmodel.State {
	if !m.stream.loaded {
		return nil
	}
	st := m.stream.vm
	return &st
}

// withServerState gives model the current state if it uses server state.
func (m Model) withServerState(model tea.Model) tea.Model {
	if p, ok := model.(serverStatePage); ok {
		return p.WithServerState(m.serverState())
	}
	return model
}

// deliverServerState hands the current state to the active page only; hidden
// pages catch up when they are switched to (see applySwitch).
func (m Model) deliverServerState() Model {
	current := m.activePage()
	if current.model == nil {
		return m
	}
	current.model = m.withServerState(current.model)
	m.setActivePage(current)
	return m
}
