package app

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/viewmodel"
)

// streamFake is a server whose snapshot and event channels the test controls.
type streamFake struct {
	*fakeServerClient

	mu        sync.Mutex
	seq       uint64
	snapshots int
	streams   []*fakeStream
}

type fakeStream struct {
	since uint64
	ctx   context.Context
	ch    chan server.Event
}

func (f *streamFake) Snapshot(context.Context) (server.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots++
	return server.Snapshot{Seq: f.seq}, nil
}

func (f *streamFake) Events(ctx context.Context, since uint64) (<-chan server.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &fakeStream{since: since, ctx: ctx, ch: make(chan server.Event, 16)}
	f.streams = append(f.streams, s)
	return s.ch, nil
}

func (f *streamFake) QueueItems(context.Context) ([]server.QueueItem, error) { return nil, nil }

func (f *streamFake) counts() (snapshots, streams, live int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.streams {
		if s.ctx.Err() == nil {
			live++
		}
	}
	return f.snapshots, len(f.streams), live
}

func (f *streamFake) stream(i int) *fakeStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[i]
}

// stubPage is a server-state page that records what the shell delivers.
type stubPage struct {
	calls int
	last  *viewmodel.State
}

func (p *stubPage) Init() tea.Cmd                       { return nil }
func (p *stubPage) Update(tea.Msg) (tea.Model, tea.Cmd) { return p, nil }
func (p *stubPage) View() tea.View                      { return tea.NewView("stub") }
func (p *stubPage) WithServerState(st *viewmodel.State) tea.Model {
	p.calls++
	p.last = st
	return p
}

// harness runs commands the way the runtime would, so a blocked event read
// keeps waiting for its channel instead of being dropped.
type harness struct {
	t    *testing.T
	m    Model
	msgs chan tea.Msg
}

// newHarness starts on the Worktrees tab: the Queue and Tickets tabs still run
// their own snapshots, which would skew the shell's counts.
func newHarness(t *testing.T, client ServerClient) *harness {
	m := newServerShellOn(t, client, nav.TabWorktrees)
	return &harness{t: t, m: m, msgs: make(chan tea.Msg, 64)}
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		switch msg := cmd().(type) {
		case nil, serverProbeTickMsg:
		case tea.BatchMsg:
			for _, c := range msg {
				h.run(c)
			}
		default:
			h.msgs <- msg
		}
	}()
}

func (h *harness) send(msg tea.Msg) {
	next, cmd := h.m.Update(msg)
	h.m = next.(Model)
	h.run(cmd)
}

// pump feeds command results back until nothing arrives for a while.
func (h *harness) pump() {
	for {
		select {
		case msg := <-h.msgs:
			h.send(msg)
		case <-time.After(150 * time.Millisecond):
			return
		}
	}
}

func (h *harness) probeUp() {
	h.send(h.m.cmdServerProbe()())
	h.pump()
}

func (h *harness) switchTo(tab nav.TabID) {
	prev := h.m.navState.Active()
	tabVS := h.m.navState.Switch(nav.ViewState{Tab: tab, WorktreeRoot: h.m.settings.ActiveWorktreePath})
	next, _ := h.m.applySwitch(tabVS, prev)
	h.m = next
}

func herdrEvent(seq uint64) server.Event {
	return server.Event{Seq: seq, Type: server.EventHerdrUnavailable}
}

func newStreamHarness(t *testing.T) (*harness, *streamFake) {
	f := &streamFake{fakeServerClient: &fakeServerClient{}, seq: 10}
	return newHarness(t, f), f
}

func TestServerStream_StartupSnapshotsAndSubscribesFromSnapshotSeq(t *testing.T) {
	h, f := newStreamHarness(t)

	h.probeUp()

	snapshots, streams, _ := f.counts()
	if snapshots != 1 || streams != 1 {
		t.Fatalf("snapshots=%d streams=%d, want 1 and 1", snapshots, streams)
	}
	if got := f.stream(0).since; got != 10 {
		t.Errorf("subscribed since %d, want 10", got)
	}
}

func TestServerStream_ReadOnlyKeepsReading(t *testing.T) {
	h, f := newStreamHarness(t)
	f.readOnly = true

	h.probeUp()
	f.stream(0).ch <- herdrEvent(11)
	h.pump()

	if h.m.stream.vm.Seq != 11 {
		t.Errorf("seq = %d, want the event applied", h.m.stream.vm.Seq)
	}
}

func TestServerStream_EventsAreReadWhateverTabIsOpen(t *testing.T) {
	for _, tab := range []nav.TabID{nav.TabTickets, nav.TabQueue, nav.TabStatus} {
		t.Run(string(tab), func(t *testing.T) {
			h, f := newStreamHarness(t)
			h.probeUp()

			h.switchTo(tab)
			f.stream(0).ch <- herdrEvent(11)
			h.pump()

			if !h.m.stream.vm.HerdrUnavailable {
				t.Errorf("event not applied while %s is open", tab)
			}
			h.switchTo(nav.TabWorktrees)
			f.stream(0).ch <- server.Event{Seq: 12, Type: server.EventHerdrAvailable}
			h.pump()
			if h.m.stream.vm.HerdrUnavailable {
				t.Errorf("event not applied after switching from %s", tab)
			}
		})
	}
}

func TestServerStream_GapAndClosedStreamEachResnapshotOnce(t *testing.T) {
	cases := map[string]func(s *fakeStream){
		"gap":    func(s *fakeStream) { s.ch <- herdrEvent(15) },
		"closed": func(s *fakeStream) { close(s.ch) },
	}
	for name, trigger := range cases {
		t.Run(name, func(t *testing.T) {
			h, f := newStreamHarness(t)
			h.probeUp()

			trigger(f.stream(0))
			h.pump()

			snapshots, streams, live := f.counts()
			if snapshots != 2 || streams != 2 || live != 1 {
				t.Errorf("snapshots=%d streams=%d live=%d, want 2, 2 and 1", snapshots, streams, live)
			}
		})
	}
}

func TestServerStream_StaleStreamMessagesAreIgnoredAfterResubscribe(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()
	f.stream(0).ch <- herdrEvent(15) // gap: re-snapshot, new subscription
	h.pump()

	f.stream(0).ch <- herdrEvent(11) // late delivery from the cancelled stream
	h.pump()

	if h.m.stream.vm.HerdrUnavailable || h.m.stream.vm.Seq != 10 {
		t.Errorf("stale event applied: %+v", h.m.stream.vm)
	}
	if _, streams, _ := f.counts(); streams != 2 {
		t.Errorf("streams = %d, want 2", streams)
	}
}

func TestServerStream_LinkDownCancelsAndUpResubscribes(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()

	f.down = true
	h.probeUp()
	if _, _, live := f.counts(); live != 0 {
		t.Fatalf("live streams = %d after link down, want 0", live)
	}

	f.down = false
	h.probeUp()
	snapshots, streams, live := f.counts()
	if snapshots != 2 || streams != 2 || live != 1 {
		t.Errorf("snapshots=%d streams=%d live=%d, want 2, 2 and 1", snapshots, streams, live)
	}
}

func TestServerStream_ActivePageGetsEachChangeAndCurrentStateOnSwitch(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()
	h.switchTo(nav.TabTickets)
	live := h.m.livePageByTab[nav.TabTickets]
	stub := &stubPage{}
	live.model = stub
	h.m.livePageByTab[nav.TabTickets] = live

	f.stream(0).ch <- herdrEvent(11)
	h.pump()
	if stub.last == nil || !stub.last.HerdrUnavailable {
		t.Fatalf("active page did not receive the change: %+v", stub.last)
	}

	h.switchTo(nav.TabStatus)
	calls := stub.calls
	f.stream(0).ch <- server.Event{Seq: 12, Type: server.EventHerdrAvailable}
	h.pump()
	if stub.calls != calls {
		t.Errorf("hidden page was delivered to")
	}

	h.switchTo(nav.TabTickets)
	if stub.last == nil || stub.last.HerdrUnavailable || stub.last.Seq != 12 {
		t.Errorf("switch did not deliver the current state: %+v", stub.last)
	}
}

func TestServerStream_BuiltPageGetsNoSnapshotMarkerThenState(t *testing.T) {
	h, _ := newStreamHarness(t)

	stub := h.m.withServerState(&stubPage{}).(*stubPage)
	if stub.calls != 1 || stub.last != nil {
		t.Fatalf("before snapshot: calls=%d last=%v, want the nil marker", stub.calls, stub.last)
	}

	h.probeUp()
	stub = h.m.withServerState(&stubPage{}).(*stubPage)
	if stub.last == nil || stub.last.Seq != 10 {
		t.Errorf("after snapshot: last = %+v, want seq 10", stub.last)
	}
}
