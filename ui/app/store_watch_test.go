package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// msgType names a message's type, for messages another package keeps private.
func msgType(msg tea.Msg) string { return reflect.TypeOf(msg).String() }

// newDownHarness is a shell whose probe found the server down, so the shell
// runs the store watch.
func newDownHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, &fakeServerClient{down: true})
	h.send(h.m.cmdServerProbe()())
	t.Cleanup(func() {
		if h.m.store.stop != nil {
			h.m.store.stop()
		}
	})
	if !h.m.store.running {
		t.Fatal("going down must start the shell's store watch")
	}
	return h
}

func TestDownMode_StoreChangeReachesTicketsTabAfterTabSwitch(t *testing.T) {
	h := newDownHarness(t)
	h.switchTo(nav.TabTickets)
	h.switchTo(nav.TabQueue)
	h.switchTo(nav.TabTickets)

	_, cmd := h.m.notifyStoreChanged()
	if cmd == nil {
		t.Fatal("the active Tickets tab was not told the store changed")
	}
	if got := msgType(cmd()); got != "tickets.epicsLoadedMsg" {
		t.Fatalf("store change produced %s, want a disk reload", got)
	}

	// The watch's own signal takes the same route.
	if _, cmd, ok := h.m.updateStoreWatch(storeChangedMsg{gen: h.m.store.gen}); !ok || cmd == nil {
		t.Fatalf("a live store signal was not handled (ok=%v)", ok)
	}
}

func TestDownMode_StaleStoreSignalsAreDropped(t *testing.T) {
	h := newDownHarness(t)
	staleGen := h.m.store.gen

	h.send(serverConnMsg{conn: ServerConn{State: ServerUp, PID: 1}})
	if h.m.store.running {
		t.Fatal("the watch kept running after the server came back")
	}
	for _, msg := range []tea.Msg{storeChangedMsg{gen: staleGen}, storePollMsg{gen: staleGen}} {
		if _, cmd, _ := h.m.updateStoreWatch(msg); cmd != nil {
			t.Fatalf("%T from a stopped watch was acted on", msg)
		}
	}
}

func TestDownMode_NoClientQueueTabShowsDownBanner(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabQueue, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := ansi.Strip(next.(Model).View().Content)
	if !strings.Contains(view, "server down — s to start") {
		t.Fatalf("no-client Queue tab:\n%s", view)
	}
}

func TestDownMode_ReloadKeyReloadsTicketsAndProbesFromQueue(t *testing.T) {
	h := newDownHarness(t)
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	r := tea.KeyPressMsg{Code: 'R', Text: "R"}

	h.switchTo(nav.TabTickets)
	h.pump()
	_, cmd := h.m.Update(r)
	if cmd == nil {
		t.Fatal(`"R" on the Tickets tab did nothing`)
	}
	if got := drain(cmd); !slices.Contains(got, "tickets.epicsLoadedMsg") || slices.Contains(got, "tickets.ResnapshotRequestedMsg") {
		t.Fatalf(`"R" on the Tickets tab produced %v, want a disk reload`, got)
	}

	h.switchTo(nav.TabQueue)
	h.pump()
	_, cmd = h.m.Update(r)
	if cmd == nil {
		t.Fatal(`"R" on the Queue tab did nothing`)
	}
	if got := drain(cmd); !slices.Contains(got, "tickets.ProbeRequestedMsg") {
		t.Fatalf(`"R" on the Queue tab produced %v, want a probe request`, got)
	}
}

// The shell answers a probe request with one handshake, and its result arms no
// further tick: the probe loop's own ticks are the only ones.
func TestDownMode_ProbeRequestIsOneShot(t *testing.T) {
	h := newDownHarness(t)

	next, cmd, ok := h.m.updateServerConn(ticketsui.ProbeRequestedMsg{})
	if !ok || cmd == nil {
		t.Fatalf("probe request not handled (ok=%v)", ok)
	}
	res, isConn := cmd().(serverConnMsg)
	if !isConn {
		t.Fatalf("probe request produced %#v, want a plain handshake result", cmd())
	}
	_, cmd, _ = next.updateServerConn(res)
	if cmd != nil {
		t.Fatal("a manual probe result armed another tick")
	}
}

func TestDownMode_NoClientIgnoresProbeRequest(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(*repo, Settings{ActiveWorktreePath: repoDir})
	if _, cmd, ok := m.updateServerConn(ticketsui.ProbeRequestedMsg{}); !ok || cmd != nil {
		t.Fatalf("no-client probe request: ok=%v cmd=%v, want handled and no command", ok, cmd)
	}
}

// drain runs cmd and any batch it holds, skipping the waits that never return
// on their own (ticks and the store-watch wait), and names what came back.
func drain(cmd tea.Cmd) []string {
	var out []string
	var walk func(tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		select {
		case msg := <-done:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, inner := range batch {
					walk(inner)
				}
			} else if msg != nil {
				out = append(out, msgType(msg))
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	walk(cmd)
	return out
}
