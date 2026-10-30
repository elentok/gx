package agents

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/contextmenu"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

// readOnlyTerminal is session a (flagged as info says) attached with the
// terminal focused.
func readOnlyTerminal(t *testing.T, info server.SessionInfo) (Model, *fakeAttach) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	m.sessionsDir = "/sessions"
	m.dial = h.dial
	m.showQueue, m.showDesign = true, true
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	info.ID, info.Name, info.Project, info.Status = "a", "alpha", "gx", idleStatus
	st := &viewmodel.State{Sessions: []server.SessionInfo{info, {ID: "b", Name: "beta", Project: "gx", Status: idleStatus}}}
	next, cmd := m.WithServerState(st, ticketsui.ServerLinkUp)
	m = next.(Model)
	if m.selectedID() != "a" {
		// Regular sessions list before the Queue and Design groups.
		m.cursor = slices.IndexFunc(m.sessions, func(s server.SessionInfo) bool { return s.ID == "a" })
		m, cmd = m.syncAttach()
	}
	m, _ = dialed(t, m, cmd)
	return update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}), h.attaches["a"]
}

// Go to pane on a PTY Queue agent: WithFocusSession selects the agent with its
// group hidden and opens its panel read-only.
func TestReadOnly_FocusSessionOpensAHiddenQueueAgentReadOnly(t *testing.T) {
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m := NewModel(&fakeSessionClient{}, ui.Settings{Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()})})
	m.sessionsDir = "/sessions"
	m.dial = h.dial
	m.opened = true
	m = update1(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	m = m.WithFocusSession("q2")
	st := &viewmodel.State{Sessions: groupSessions()}
	for i := range st.Sessions {
		st.Sessions[i].Status = idleStatus
	}
	next, cmd := m.WithServerState(st, ticketsui.ServerLinkUp)
	m = next.(Model)
	m, _ = dialed(t, m, cmd)
	if m.showQueue || m.selectedID() != "q2" {
		t.Fatalf("showQueue=%v selected=%q, want the hidden group's q2 selected", m.showQueue, m.selectedID())
	}
	if a := m.attach; a == nil || a.id != "q2" || !a.readOnly {
		t.Fatalf("attach = %+v, want q2 read-only", a)
	}
}

func TestReadOnly_QueueAndDesignAgentsDropKeysAndPaste(t *testing.T) {
	for name, info := range map[string]server.SessionInfo{"queue": {Queue: true}, "design": {Design: true}} {
		t.Run(name, func(t *testing.T) {
			m, fa := readOnlyTerminal(t, info)
			if !m.terminalFocused {
				t.Fatal("enter did not focus the terminal")
			}
			m = update1(t, m, letter("x"))
			m = update1(t, m, tea.PasteMsg{Content: "pasted"})
			if len(fa.keys) != 0 || len(fa.pastes) != 0 {
				t.Fatalf("read-only agent got keys %v and pastes %v", fa.keys, fa.pastes)
			}
		})
	}
}

func TestReadOnly_PlainSessionAndFixAgentAreWritable(t *testing.T) {
	m, fa := readOnlyTerminal(t, server.SessionInfo{})
	m = update1(t, m, letter("x"))
	update1(t, m, tea.PasteMsg{Content: "pasted"})
	if len(fa.keys) != 1 || !slices.Equal(fa.pastes, []string{"pasted"}) {
		t.Fatalf("plain session got keys %v and pastes %v", fa.keys, fa.pastes)
	}
}

func TestReadOnly_WheelAndCopyModeStillWork(t *testing.T) {
	m, fa := readOnlyTerminal(t, server.SessionInfo{Queue: true})
	fa.scrolledFrame(0, 100, 0, "$ claude")
	m, _ = read(t, m, readAttach(m.attach))
	m = update1(t, m, wheel(sidebarWidth+5, 4, tea.MouseWheelUp))
	if !slices.Contains(fa.log, "scroll:3") {
		t.Fatalf("wheel sent %v, want scroll:3", fa.log)
	}
	m, _ = leaderThen(t, m, letter("["))
	if m.copy == nil {
		t.Fatal("leader [ did not enter copy mode")
	}
}

func TestReadOnly_UnlockConfirmNamesTheKindAndDefaultsToCancel(t *testing.T) {
	for _, tc := range []struct {
		info server.SessionInfo
		want string
	}{
		{server.SessionInfo{Queue: true}, "gx is driving this Queue agent"},
		{server.SessionInfo{Design: true}, "This Design agent is waiting on gx-design"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			m, fa := readOnlyTerminal(t, tc.info)
			m, _ = leaderThen(t, m, letter("i"))
			if !m.confirm.IsOpen || !strings.Contains(plainView(m), tc.want) {
				t.Fatalf("unlock did not ask %q:\n%s", tc.want, plainView(m))
			}
			// Enter takes the default, which is Cancel.
			m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			m = update1(t, m, letter("x"))
			if m.confirm.IsOpen || len(fa.keys) != 0 {
				t.Fatalf("Enter on the dialog unlocked: keys %v", fa.keys)
			}
		})
	}
}

func TestReadOnly_ConfirmedUnlockLetsKeysThroughUntilTheNextOpen(t *testing.T) {
	m, fa := readOnlyTerminal(t, server.SessionInfo{Queue: true})
	opened := m.attach
	m, _ = leaderThen(t, m, letter("i"))
	next, cmd := m.Update(letter("y"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("confirming returned no cmd")
	}
	m = update1(t, m, cmd())
	m = update1(t, m, letter("x"))
	if len(fa.keys) != 1 {
		t.Fatalf("unlocked agent got keys %v, want one", fa.keys)
	}

	here := m.cursor
	m.cursor = 1 - here
	m, _ = m.syncAttach()
	m.cursor = here
	m, _ = m.syncAttach()
	if m.attach == opened || m.attach == nil || !m.attach.readOnly {
		t.Fatal("selecting the agent again did not open it read-only")
	}
}

func TestReadOnly_QueueAgentHidesStopRemoveAndRestore(t *testing.T) {
	m, _ := readOnlyTerminal(t, server.SessionInfo{Queue: true})
	s, _ := m.selected()
	has := func(label string) bool {
		return slices.ContainsFunc(sessionMenuItems(s), func(it contextmenu.Item[server.SessionInfo]) bool { return it.Label == label })
	}
	for _, label := range []string{"Stop", "Remove", "Restore session"} {
		if has(label) {
			t.Errorf("the Queue agent menu has %q", label)
		}
	}
	if !has("Unlock input") {
		t.Error("the Queue agent menu has no Unlock input")
	}
	m, _ = leaderThen(t, m, letter("x"))
	if m.confirm.IsOpen {
		t.Fatal("leader x opened a Stop confirm on a Queue agent")
	}
}
