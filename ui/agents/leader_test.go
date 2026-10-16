package agents

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var (
	ctrlA = tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	ctrlB = tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
)

// leaderThen presses the leader and then k, returning the second key's cmd.
func leaderThen(t *testing.T, m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	m = update1(t, m, ctrlA)
	if !m.leaderPending {
		t.Fatal("the leader did not start leader mode")
	}
	next, cmd := m.Update(k)
	return settle(next.(Model)), cmd
}

func letter(k string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(k[0]), Text: k} }

// focusedTerminal is session a attached and the terminal focused.
func focusedTerminal(t *testing.T) (Model, *fakeAttach) {
	t.Helper()
	h := &fakeHolders{attaches: map[string]*fakeAttach{}}
	m, cmd := attachModel(t, h)
	m, _ = dialed(t, m, cmd)
	m = update1(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.terminalFocused {
		t.Fatal("enter did not focus the terminal")
	}
	return m, h.attaches["a"]
}

func TestLeader_NextAndPreviousSession(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	m, _ = leaderThen(t, m, letter("n"))
	if m.selectedID() != "b" {
		t.Fatalf("leader n selected %q, want b", m.selectedID())
	}
	m, _ = leaderThen(t, m, letter("p"))
	if m.selectedID() != "a" {
		t.Fatalf("leader p selected %q, want a", m.selectedID())
	}
}

func attentionModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	st := &viewmodel.State{Sessions: []server.SessionInfo{
		{ID: "a", Project: "gx", Status: idleStatus},
		{ID: "b", Project: "gx", Status: idleStatus, Unread: server.SessionUnread{Unread: true}},
		{ID: "c", Project: "gx", Status: server.SessionStatus{State: server.SessionBlocked}},
	}}
	return withState(t, m, st, ticketsui.ServerLinkUp)
}

func TestLeader_UGoesToActionableBeforeUnread(t *testing.T) {
	m := attentionModel(t)
	m, _ = leaderThen(t, m, letter("u"))
	if m.selectedID() != "c" {
		t.Fatalf("leader u selected %q, want the Actionable c", m.selectedID())
	}
	m, _ = leaderThen(t, m, letter("u"))
	if m.selectedID() != "c" {
		t.Fatalf("leader u from the only Actionable selected %q, want c", m.selectedID())
	}
}

func TestLeader_UFallsBackToUnread(t *testing.T) {
	m := attentionModel(t)
	m.sessions[2].Status.State = server.SessionIdle
	m, _ = leaderThen(t, m, letter("u"))
	if m.selectedID() != "b" {
		t.Fatalf("leader u selected %q, want the Unread b", m.selectedID())
	}
}

func TestSidebar_PlainUGoesToAttention(t *testing.T) {
	m := press(t, attentionModel(t), "u")
	if m.selectedID() != "c" {
		t.Fatalf("u selected %q, want c", m.selectedID())
	}
}

func TestLeader_TwiceSendsOneLeaderKey(t *testing.T) {
	m, a := focusedTerminal(t)
	m, _ = leaderThen(t, m, ctrlA)
	if m.leaderPending {
		t.Fatal("leader twice left leader mode on")
	}
	if len(a.keys) != 1 || a.keys[0].Code != 'a' || a.keys[0].Mod != tea.ModCtrl {
		t.Fatalf("sent %+v, want one ctrl+a", a.keys)
	}
}

func TestLeader_EscAndUnboundKeyEndLeaderModeSendingNothing(t *testing.T) {
	for name, k := range map[string]tea.KeyPressMsg{"esc": esc, "unbound": letter("Z")} {
		t.Run(name, func(t *testing.T) {
			m, a := focusedTerminal(t)
			m, _ = leaderThen(t, m, k)
			if m.leaderPending || len(a.keys) != 0 {
				t.Fatalf("pending=%v sent=%+v, want leader mode over and nothing sent", m.leaderPending, a.keys)
			}
			m = update1(t, m, letter("x"))
			if len(a.keys) != 1 {
				t.Fatalf("the key after leader mode was not sent: %+v", a.keys)
			}
		})
	}
}

func TestLeader_WhichKeyBoxWhileWaiting(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	if strings.Contains(plainView(m), "next session") {
		t.Fatal("which-key box shown before the leader")
	}
	m = update1(t, m, ctrlA)
	if view := plainView(m); !strings.Contains(view, "next session") || !strings.Contains(view, "toggle sidebar") {
		t.Fatalf("which-key box missing while waiting:\n%s", view)
	}
	m = update1(t, m, esc)
	if strings.Contains(plainView(m), "next session") {
		t.Fatal("which-key box still shown after esc")
	}
}

func TestLeader_EveryConfiguredKeyWorks(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	m.settings.Agents.LeaderKey = []string{"ctrl+a", "ctrl+b"}
	m = update1(t, m, ctrlB)
	if !m.leaderPending {
		t.Fatal("the second configured leader did not start leader mode")
	}
	m = update1(t, m, letter("n"))
	if m.selectedID() != "b" {
		t.Fatalf("ctrl+b n selected %q, want b", m.selectedID())
	}
	m, _ = leaderThen(t, m, letter("p"))
	if m.selectedID() != "a" {
		t.Fatal("ctrl+a stopped working with a list configured")
	}
}

func TestLeader_InSidebarFocusTakesTheNextKeyFromTheShell(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	m = update1(t, m, ctrlA)
	if !m.InputFocused() {
		t.Fatal("leader mode must keep the shell from reading the next key")
	}
}

func TestLeader_TabMovesFocus(t *testing.T) {
	m, a := focusedTerminal(t)
	m, _ = leaderThen(t, m, tab)
	if m.terminalFocused {
		t.Fatal("leader tab did not move focus to the sidebar")
	}
	m, _ = leaderThen(t, m, tab)
	if !m.terminalFocused {
		t.Fatal("leader tab did not move focus to the terminal")
	}
	if len(a.keys) != 0 {
		t.Fatalf("leader tab reached the session: %+v", a.keys)
	}
}

func TestLeader_DigitSwitchesTab(t *testing.T) {
	m, _ := focusedTerminal(t)
	_, cmd := leaderThen(t, m, letter("2"))
	if cmd == nil {
		t.Fatal("leader 2 returned no command")
	}
	vs, ok := nav.IsSwitch(cmd())
	if !ok || vs.Tab != nav.TabLog {
		t.Fatalf("leader 2 switched to %+v (ok=%v), want the log tab", vs, ok)
	}
}

func TestLeader_QGoesBack(t *testing.T) {
	m, _ := focusedTerminal(t)
	_, cmd := leaderThen(t, m, letter("q"))
	if cmd == nil || !nav.IsBack(cmd()) {
		t.Fatal("leader q did not go back")
	}
}

func TestLeader_ToggleSidebarAndHelp(t *testing.T) {
	m, _ := attachModel(t, &fakeHolders{attaches: map[string]*fakeAttach{}})
	m, _ = leaderThen(t, m, letter("b"))
	if !m.hideSidebar {
		t.Fatal("leader b did not hide the sidebar")
	}
	m, _ = leaderThen(t, m, letter("?"))
	if !m.help.IsOpen || !strings.Contains(plainView(m), "Keybindings") {
		t.Fatal("leader ? did not open help")
	}
	m = update1(t, m, esc)
	if m.help.IsOpen {
		t.Fatal("esc did not close help")
	}
}
