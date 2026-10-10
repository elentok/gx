package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

// streamFake is a server whose snapshot and event channels the test controls.
type streamFake struct {
	*fakeServerClient

	mu        sync.Mutex
	seq       uint64
	tickets   []server.TicketInfo
	snapErr   error
	snapshots int
	streams   []*fakeStream
	mode      string
	queue     []server.QueueItem
	queueGets int
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
	if f.snapErr != nil {
		return server.Snapshot{}, f.snapErr
	}
	return server.Snapshot{Seq: f.seq, Tickets: f.tickets, Mode: f.mode}, nil
}

func (f *streamFake) Events(ctx context.Context, since uint64) (<-chan server.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &fakeStream{since: since, ctx: ctx, ch: make(chan server.Event, 16)}
	f.streams = append(f.streams, s)
	return s.ch, nil
}

func (f *streamFake) QueueItems(context.Context) ([]server.QueueItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queueGets++
	return f.queue, nil
}

func (f *streamFake) Budget(context.Context) (server.BudgetStatus, error) {
	return server.BudgetStatus{}, nil
}

// fetches is how many snapshots and queue reads the server has answered.
func (f *streamFake) fetches() (snapshots, queueGets int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshots, f.queueGets
}

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
func (p *stubPage) WithServerState(st *viewmodel.State) (tea.Model, tea.Cmd) {
	p.calls++
	p.last = st
	return p, nil
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
		// A spinner tick re-arms itself, so feeding it back would never go idle.
		case nil, serverProbeTickMsg, spinner.TickMsg:
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
	next, cmd := h.m.applySwitch(tabVS, prev)
	h.m = next
	h.run(cmd)
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

	got, _ := h.m.withServerState(&stubPage{})
	stub := got.(*stubPage)
	if stub.calls != 1 || stub.last != nil {
		t.Fatalf("before snapshot: calls=%d last=%v, want the nil marker", stub.calls, stub.last)
	}

	h.probeUp()
	got, _ = h.m.withServerState(&stubPage{})
	stub = got.(*stubPage)
	if stub.last == nil || stub.last.Seq != 10 {
		t.Errorf("after snapshot: last = %+v, want seq 10", stub.last)
	}
}

// ticketsTab builds the Tickets tab the way the shell does and sizes it.
func ticketsTab(h *harness) ticketsui.Model {
	h.t.Helper()
	tm := h.m.newTicketsModel(h.m.settings.ActiveWorktreePath, h.m.settings.Settings)
	withState, _ := h.m.withServerState(tm)
	next, _ := withState.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(ticketsui.Model)
}

func TestServerStream_OneStreamWhateverTabsAreVisited(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()

	h.switchTo(nav.TabTickets)
	h.pump()
	h.switchTo(nav.TabWorktrees)
	h.switchTo(nav.TabTickets)
	h.pump()

	snapshots, streams, live := f.counts()
	if snapshots != 1 || streams != 1 || live != 1 {
		t.Errorf("snapshots=%d streams=%d live=%d, want 1 each", snapshots, streams, live)
	}
}

func TestServerStream_TicketsTabLoadingThenCurrentState(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = []server.TicketInfo{{Address: "gx:alpha/01", Title: "Zeta ticket", Status: "open"}}

	if view := ticketsTab(h).View().Content; !strings.Contains(view, "loading…") || strings.Contains(view, "Zeta ticket") {
		t.Fatalf("before the first snapshot the tab must show loading…:\n%s", view)
	}

	h.probeUp()
	// A tab built (or rebuilt after a worktree change) now starts current.
	if view := ticketsTab(h).View().Content; !strings.Contains(view, "Zeta ticket") {
		t.Fatalf("rebuilt tab missing delivered state:\n%s", view)
	}
}

func TestServerStream_EventWhileQueueOpenShowsOnTicketsAfterSwitchBack(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()
	h.switchTo(nav.TabTickets)
	live := h.m.livePageByTab[nav.TabTickets]
	stub := &stubPage{}
	live.model = stub
	h.m.livePageByTab[nav.TabTickets] = live

	h.switchTo(nav.TabQueue)
	f.stream(0).ch <- herdrEvent(11)
	h.pump()
	h.switchTo(nav.TabTickets)

	if stub.last == nil || !stub.last.HerdrUnavailable {
		t.Errorf("event that arrived meanwhile is missing: %+v", stub.last)
	}
}

// toastTexts runs cmd (flattening batches) and returns the toasts it emits. A
// command that blocks, like the next event read, counts as emitting none.
func toastTexts(cmd tea.Cmd) []notify.NotifyMsg {
	if cmd == nil {
		return nil
	}
	res := make(chan tea.Msg, 1)
	go func() { res <- cmd() }()
	var got tea.Msg
	select {
	case got = <-res:
	case <-time.After(100 * time.Millisecond):
		return nil
	}
	switch msg := got.(type) {
	case notify.NotifyMsg:
		return []notify.NotifyMsg{msg}
	case tea.BatchMsg:
		var out []notify.NotifyMsg
		for _, c := range msg {
			out = append(out, toastTexts(c)...)
		}
		return out
	}
	return nil
}

func TestServerStream_ParkToastsOnLiveEventsOnly(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "needs-repair"}}
	h.probeUp() // the snapshot holds a parked ticket: state, not news

	park := server.Event{Seq: 11, Type: server.EventIterationParked, Address: "gx:alpha/01"}
	events := h.m.stream.streamEvents
	next, cmd := h.m.Update(streamEventMsg{ev: park, events: events})
	if got := toastTexts(cmd); len(got) != 1 || got[0].Kind != notify.KindWarning {
		t.Fatalf("live park toasts = %+v", got)
	}

	// The same event replayed after a reconnect is a duplicate: no toast.
	_, cmd = next.(Model).Update(streamEventMsg{ev: park, events: events})
	if got := toastTexts(cmd); len(got) != 0 {
		t.Fatalf("replayed event toasted: %+v", got)
	}
}

func TestServerStream_SnapshotNeverToastsButFailureDoes(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "needs-repair"}}
	next, cmd := h.m.Update(streamSnapshotMsg{snap: server.Snapshot{Seq: 3, Tickets: f.tickets}})
	h.m = next.(Model)
	for _, got := range toastTexts(cmd) {
		if got.Kind == notify.KindWarning { // the unregistered-project hint is Info
			t.Fatalf("snapshot toasted: %+v", got)
		}
	}

	h.m.stream.snapshotting = true
	_, cmd = h.m.Update(streamSnapshotMsg{gen: h.m.stream.gen, err: context.DeadlineExceeded})
	if got := toastTexts(cmd); len(got) != 1 || got[0].Kind != notify.KindError {
		t.Fatalf("failed snapshot toasts = %+v", got)
	}
}

func TestServerStream_FailureStreakToastsOnceAndRetriesOnProbeTick(t *testing.T) {
	h, f := newStreamHarness(t)
	f.snapErr = context.DeadlineExceeded
	h.m.serverConn = ServerConn{State: ServerUp}
	fail := func() []notify.NotifyMsg {
		h.m.stream.snapshotting = true
		next, cmd := h.m.Update(streamSnapshotMsg{gen: h.m.stream.gen, err: f.snapErr})
		h.m = next.(Model)
		return toastTexts(cmd)
	}
	if got := fail(); len(got) != 1 {
		t.Fatalf("first failure toasts = %+v", got)
	}
	if !h.m.streamWanted() {
		t.Fatal("a failed snapshot must leave the shell wanting a stream, for the next probe tick")
	}
	if got := fail(); len(got) != 0 {
		t.Fatalf("same streak toasted again: %+v", got)
	}

	// A success ends the streak, so the next failure toasts again.
	next, _ := h.m.Update(streamSnapshotMsg{gen: h.m.stream.gen})
	h.m = next.(Model)
	if got := fail(); len(got) != 1 {
		t.Fatalf("failure after a success toasts = %+v", got)
	}
}

func TestServerStream_FailedResnapshotWithLiveStreamRetriesOncePerProbeTick(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()
	snapshots := func() int { n, _ := f.fetches(); return n }

	f.mu.Lock()
	f.snapErr = context.DeadlineExceeded
	f.mu.Unlock()
	base := snapshots()
	h.send(ticketsui.ResnapshotRequestedMsg{})
	h.pump()
	if got := snapshots() - base; got != 1 {
		t.Fatalf("snapshots after failed R = %d, want 1", got)
	}
	if _, _, live := f.counts(); live != 1 {
		t.Fatalf("live streams = %d, want the old stream kept", live)
	}

	h.probeUp()
	if got := snapshots() - base; got != 2 {
		t.Fatalf("snapshots after one probe tick = %d, want 2", got)
	}
	h.probeUp()
	if got := snapshots() - base; got != 3 {
		t.Fatalf("snapshots after two probe ticks = %d, want 3", got)
	}

	f.mu.Lock()
	f.snapErr = nil
	f.mu.Unlock()
	h.probeUp()
	recovered := snapshots()
	if recovered-base != 4 {
		t.Fatalf("snapshots after recovery = %d, want 4", recovered-base)
	}
	h.probeUp()
	if got := snapshots(); got != recovered {
		t.Fatalf("snapshot retried after success: %d -> %d", recovered, got)
	}
}

func TestServerStream_NoRetryWhileLinkDown(t *testing.T) {
	h, _ := newStreamHarness(t)
	h.probeUp()
	h.m.stream.snapshotFailing = true
	h.m.serverConn = ServerConn{State: ServerDown}
	if h.m.streamWanted() {
		t.Fatal("a failing snapshot must not be retried while the link is down")
	}
}

func TestServerStream_UnregisteredHintComesFromShellOnFirstSnapshotOnly(t *testing.T) {
	h, _ := newStreamHarness(t) // the temp worktree is not a registered project
	hints := func(cmd tea.Cmd) int {
		n := 0
		for _, got := range toastTexts(cmd) {
			if got.Kind == notify.KindInfo && strings.Contains(got.Message,"not in a registered project") {
				n++
			}
		}
		return n
	}
	next, cmd := h.m.Update(streamSnapshotMsg{snap: server.Snapshot{Seq: 3}})
	if got := hints(cmd); got != 1 {
		t.Fatalf("first snapshot hints = %d, want 1", got)
	}
	h.m = next.(Model)
	h.m.stream.snapshotting = true
	_, cmd = h.m.Update(streamSnapshotMsg{gen: h.m.stream.gen, snap: server.Snapshot{Seq: 4}})
	if got := hints(cmd); got != 0 {
		t.Fatalf("later snapshot hints = %d, want 0", got)
	}
}

func TestServerStream_RefreshAsksForOneSnapshotPlusQueue(t *testing.T) {
	h, f := newStreamHarness(t)
	h.probeUp()
	before, _, _ := f.counts()
	h.send(ticketsui.ResnapshotRequestedMsg{})
	h.send(ticketsui.ResnapshotRequestedMsg{}) // coalesces into the one in flight
	h.pump()
	after, _, live := f.counts()
	if after-before != 1 {
		t.Fatalf("snapshots = %d, want 1", after-before)
	}
	if live != 1 {
		t.Fatalf("live streams = %d, want 1", live)
	}
}
