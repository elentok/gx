package agents

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// fixtureSessions interleaves projects so grouping is exercised, and covers
// every status, both marks, +bg and a Session ticket.
func fixtureSessions() []server.SessionInfo {
	return []server.SessionInfo{
		{ID: "s1", Name: "fix login", Project: "gx", Ticket: "gx:auth/03",
			Status: server.SessionStatus{State: server.SessionWorking, Background: true},
			Unread: server.SessionUnread{Unread: true}},
		{ID: "s2", Name: "nvim", Project: "dotfiles",
			Status: server.SessionStatus{State: server.SessionExited}, Exit: server.SessionExitCrashed,
			Actionable: server.SessionActionable{Reason: "crashed"}},
		{ID: "s3", Name: "docs", Project: "gx", Status: server.SessionStatus{State: server.SessionIdle}},
		{ID: "s4", Name: "review", Project: "gx", Status: server.SessionStatus{State: server.SessionBlocked},
			Unread: server.SessionUnread{Unread: true}},
		{ID: "s5", Name: "zsh", Project: "dotfiles",
			Status: server.SessionStatus{State: server.SessionExited}, Exit: server.SessionExitClean},
		{ID: "s6", Name: "limits", Project: "dotfiles", Status: server.SessionStatus{State: server.SessionRateLimited}},
		{ID: "s7", Project: "dotfiles", Status: server.SessionStatus{State: server.SessionStarting}},
	}
}

func sidebarModel(t *testing.T, nerd bool, sessions []server.SessionInfo, width, height int) Model {
	t.Helper()
	m := NewModel(&fakeSessionClient{}, ui.Settings{UseNerdFontIcons: nerd})
	// Start on the first session in the sidebar, past the opening focus.
	m.opened = true
	m = withState(t, m, &viewmodel.State{Sessions: sessions}, ticketsui.ServerLinkUp)
	return update1(t, m, tea.WindowSizeMsg{Width: width, Height: height})
}

func update1(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return settle(next.(Model))
}

// settle waits until the attach's queued writes reached the holder.
func settle(m Model) Model {
	if a := m.attach; a != nil && a.w != nil {
		a.w.pending.Wait()
	}
	return m
}

func press(t *testing.T, m Model, k string) Model {
	t.Helper()
	code := rune(k[0])
	switch k {
	case "down":
		code = tea.KeyDown
	case "up":
		code = tea.KeyUp
	default:
		return update1(t, m, tea.KeyPressMsg{Code: code, Text: k})
	}
	return update1(t, m, tea.KeyPressMsg{Code: code})
}

func TestSidebar_Golden(t *testing.T) {
	for name, tc := range map[string]struct {
		nerd, terminalFocused bool
	}{
		"sidebar_nerd_focused":   {nerd: true},
		"sidebar_nerd_unfocused": {nerd: true, terminalFocused: true},
		"sidebar_ascii_focused":  {},
	} {
		t.Run(name, func(t *testing.T) {
			m := sidebarModel(t, tc.nerd, fixtureSessions(), 70, 14)
			m.terminalFocused = tc.terminalFocused
			assertGolden(t, name, m.View().Content)
		})
	}
}

func assertGolden(t *testing.T, name, text string) {
	t.Helper()
	golden := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if text != string(want) {
		t.Errorf("render mismatch (run with -update to accept)\ngot:\n%s\nwant:\n%s", ansi.Strip(text), ansi.Strip(string(want)))
	}
}

func TestSidebar_GroupsByProjectInCreationOrder(t *testing.T) {
	m := sidebarModel(t, false, fixtureSessions(), 70, 14)
	var names []string
	for _, s := range m.sessions {
		names = append(names, s.ID)
	}
	if got := strings.Join(names, " "); got != "s1 s3 s4 s2 s5 s6 s7" {
		t.Fatalf("sidebar order = %q", got)
	}
}

func TestSidebar_JKAndArrowsMoveTheSelection(t *testing.T) {
	m := sidebarModel(t, false, fixtureSessions(), 70, 14)
	for _, step := range []struct {
		key  string
		want string
	}{
		{"j", "s3"}, {"down", "s4"}, {"k", "s3"}, {"up", "s1"}, {"k", "s1"},
	} {
		m = press(t, m, step.key)
		if got := m.selectedID(); got != step.want {
			t.Fatalf("after %s: selected %s, want %s", step.key, got, step.want)
		}
	}
}

func TestSidebar_SelectionFollowsTheSessionAcrossUpdates(t *testing.T) {
	m := press(t, sidebarModel(t, false, fixtureSessions(), 70, 14), "j")
	sessions := append([]server.SessionInfo{{ID: "s0", Project: "gx"}}, fixtureSessions()...)
	m = withState(t, m, &viewmodel.State{Sessions: sessions}, ticketsui.ServerLinkUp)
	if got := m.selectedID(); got != "s3" {
		t.Fatalf("selected %s after an update, want s3", got)
	}
}

func TestSidebar_BTogglesTheSidebar(t *testing.T) {
	m := sidebarModel(t, false, fixtureSessions(), 70, 14)
	m = press(t, m, "b")
	if strings.Contains(ansi.Strip(m.View().Content), "5 live") {
		t.Fatal("sidebar still shown after b")
	}
	m = press(t, m, "b")
	if !strings.Contains(ansi.Strip(m.View().Content), "5 live") {
		t.Fatal("sidebar not back after a second b")
	}
}

func TestSidebar_SpinnerTicksOnlyWhileAWorkingSessionIsVisible(t *testing.T) {
	idle := []server.SessionInfo{{ID: "a", Project: "gx", Status: server.SessionStatus{State: server.SessionIdle}}}
	working := []server.SessionInfo{{ID: "w", Project: "gx", Status: server.SessionStatus{State: server.SessionWorking}}}

	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	m.sessionsDir = "" // no attach, so a cmd can only be a tick
	if _, cmd := m.WithServerState(&viewmodel.State{Sessions: idle}, ticketsui.ServerLinkUp); cmd != nil {
		t.Fatal("tick scheduled with no Working session")
	}
	next, cmd := m.WithServerState(&viewmodel.State{Sessions: working}, ticketsui.ServerLinkUp)
	if cmd == nil {
		t.Fatal("no tick scheduled with a Working session on screen")
	}
	m = next.(Model)

	hidden := press(t, m, "b")
	if _, cmd := hidden.Update(cmd()); cmd != nil {
		t.Fatal("tick re-armed with the sidebar hidden")
	}

	// Many idle sessions push the only Working one below the sidebar's fold.
	var offscreen []server.SessionInfo
	for range 20 {
		offscreen = append(offscreen, idle[0])
	}
	offscreen = append(offscreen, working...)
	fresh := NewModel(&fakeSessionClient{}, ui.Settings{})
	fresh.sessionsDir = ""
	sized := update1(t, fresh, tea.WindowSizeMsg{Width: 70, Height: 10})
	if _, cmd := sized.WithServerState(&viewmodel.State{Sessions: offscreen}, ticketsui.ServerLinkUp); cmd != nil {
		t.Fatal("tick scheduled for a Working session scrolled off screen")
	}
}

func TestJump_SpinnerTicksForAWorkingSessionWithTheSidebarHidden(t *testing.T) {
	working := []server.SessionInfo{{ID: "w", Name: "worker", Project: "gx", Status: server.SessionStatus{State: server.SessionWorking}}}
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	m.sessionsDir = ""
	m.hideSidebar = true
	m = withState(t, m, &viewmodel.State{Sessions: working}, ticketsui.ServerLinkUp)
	if m.ticking {
		t.Fatal("ticking with the sidebar hidden and no popup")
	}

	next, tick := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(Model)
	if tick == nil {
		t.Fatal("no tick scheduled with the popup showing a Working session")
	}
	next, cmd := m.Update(tick())
	if cmd == nil {
		t.Fatal("tick not re-armed while the popup shows a Working session")
	}
	m = next.(Model)

	filtered := typeText(t, m, "zzz")
	if _, cmd := filtered.Update(tick()); cmd != nil {
		t.Fatal("tick re-armed with the Working session filtered out")
	}
	closed := update1(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if _, cmd := closed.Update(tick()); cmd != nil {
		t.Fatal("tick re-armed after the popup closed")
	}
}
