package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

var ctrlO = tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}

func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func typeText(m Model, text string) Model {
	for _, r := range text {
		m = pressKey(m, r)
	}
	return m
}

// switcherApp is a 120x40 shell on the worktrees view with a recorder page.
func switcherApp(t *testing.T) (Model, *mouseRecorderStub) {
	t.Helper()
	return withStubPage(newAppForMouseTest(t))
}

func openedSwitcher(t *testing.T) (Model, *mouseRecorderStub) {
	t.Helper()
	m, stub := switcherApp(t)
	m, _ = step(m, ctrlO)
	if !m.switcher.isOpen {
		t.Fatal("ctrl+o did not open the switcher")
	}
	stub.received = nil
	return m, stub
}

func switcherLabels(m Model) []string {
	var out []string
	for _, match := range m.switcherPicker().Matches() {
		out = append(out, match.Item.Label)
	}
	return out
}

// emitsInfo reports whether cmd (or a batch inside it) emits an info toast
// with text.
func emitsInfo(cmd tea.Cmd, text string) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		return slices.ContainsFunc(msg, func(c tea.Cmd) bool { return emitsInfo(c, text) })
	case notify.NotifyMsg:
		return strings.Contains(msg.Message, text)
	}
	return false
}

func TestSwitcher_CtrlOOpensWithAllViewsGrouped(t *testing.T) {
	t.Parallel()
	m, _ := openedSwitcher(t)

	want := []string{"worktrees", "log", "status", "stash", "prs", "tickets", "queue", "agents"}
	if got := switcherLabels(m); !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	items := m.switcher.picker.Items
	if items[0].Group != "git" || items[4].Group != "git" || items[5].Group != "agents" || items[7].Group != "agents" {
		t.Fatalf("groups = %+v", items)
	}
	if !items[0].Current || items[1].Current {
		t.Fatalf("current should be worktrees only: %+v", items)
	}
}

func TestSwitcher_TypingFiltersAndEnterSwitches(t *testing.T) {
	t.Parallel()
	m, stub := openedSwitcher(t)

	m = typeText(m, "que")
	if got := switcherLabels(m); !slices.Equal(got, []string{"queue"}) {
		t.Fatalf("matches = %v, want [queue]", got)
	}
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.switcher.isOpen {
		t.Fatal("enter should close the switcher")
	}
	if got := m.navState.ActiveTab(); got != nav.TabQueue {
		t.Fatalf("active tab = %q, want queue", got)
	}
	if len(stub.received) != 0 {
		t.Fatalf("keys reached the page: %#v", stub.received)
	}
}

func TestSwitcher_DigitsFilterInsteadOfSwitching(t *testing.T) {
	t.Parallel()
	m, _ := openedSwitcher(t)

	m = pressKey(m, '2')
	if !m.switcher.isOpen || m.switcher.picker.Filter != "2" {
		t.Fatalf("switcher = %+v, want open with filter 2", m.switcher)
	}
	if got := m.navState.ActiveTab(); got != nav.TabWorktrees {
		t.Fatalf("active tab = %q, want unchanged", got)
	}
}

func TestSwitcher_EnterOnNoMatchesDoesNothing(t *testing.T) {
	t.Parallel()
	m, _ := openedSwitcher(t)

	m = typeText(m, "zzz")
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.switcher.isOpen {
		t.Fatal("enter on no matches should keep the switcher open")
	}
	if got := m.navState.ActiveTab(); got != nav.TabWorktrees {
		t.Fatalf("active tab = %q, want unchanged", got)
	}
}

func TestSwitcher_EscAndCtrlOClose(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, ctrlO} {
		m, stub := openedSwitcher(t)
		m, _ = step(m, key)
		if m.switcher.isOpen {
			t.Fatalf("%s should close the switcher", key)
		}
		if len(stub.received) != 0 {
			t.Fatalf("%s reached the page", key)
		}
	}
}

func TestSwitcher_CtrlCClosesAndForceQuits(t *testing.T) {
	t.Parallel()
	m, stub := openedSwitcher(t)

	m, cmd := step(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.switcher.isOpen {
		t.Fatal("ctrl+c should close the switcher")
	}
	if cmd == nil || !nav.IsForceQuit(cmd()) {
		t.Fatal("ctrl+c should yield the force-quit command")
	}
	if len(stub.received) != 0 {
		t.Fatalf("ctrl+c reached the page: %#v", stub.received)
	}
}

func TestSwitcher_ViewShowsHintAndNoCursor(t *testing.T) {
	t.Parallel()
	m, stub := switcherApp(t)
	stub.cursor = tea.NewCursor(1, 1)
	if m.View().Cursor == nil {
		t.Fatal("the page cursor should show while the switcher is closed")
	}

	m, _ = step(m, ctrlO)
	v := m.View()
	if v.Cursor != nil {
		t.Fatalf("cursor = %+v, want none while the switcher is open", v.Cursor)
	}
	if !strings.Contains(ansi.Strip(v.Content), "enter select") {
		t.Fatal("the switcher should show its key hint")
	}
}

func TestSwitcher_TickStillReachesPage(t *testing.T) {
	t.Parallel()
	m, stub := openedSwitcher(t)

	m, _ = step(m, fakeTickMsg{})
	if len(stub.received) != 1 || stub.received[0] != (fakeTickMsg{}) {
		t.Fatalf("page received %#v, want the tick", stub.received)
	}
	if !m.switcher.isOpen {
		t.Fatal("a tick should not close the switcher")
	}
}

type terminalFocusedStub struct{ mouseRecorderStub }

func (s *terminalFocusedStub) TerminalFocused() bool { return true }

func TestSwitcher_CtrlOGoesToFocusedTerminal(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)
	stub := &terminalFocusedStub{}
	live := m.livePageByTab[nav.TabWorktrees]
	live.model = stub
	m.livePageByTab[nav.TabWorktrees] = live

	m, _ = step(m, ctrlO)
	if m.switcher.isOpen {
		t.Fatal("switcher opened over a focused terminal")
	}
	if len(stub.received) != 1 || stub.received[0] != tea.Msg(ctrlO) {
		t.Fatalf("page received %#v, want ctrl+o", stub.received)
	}
}

func TestSwitcher_ModalOpenGuard(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)
	live := m.livePageByTab[nav.TabWorktrees]
	live.model = &modalOpenStub{}
	m.livePageByTab[nav.TabWorktrees] = live

	for _, msg := range []tea.Msg{ctrlO, nav.OpenSwitcher()(), tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft}} {
		next, cmd := step(m, msg)
		if next.switcher.isOpen {
			t.Fatalf("%T opened the switcher over a page modal", msg)
		}
		if !emitsInfo(cmd, "close the modal first") {
			t.Fatalf("%T: want a close-the-modal-first toast", msg)
		}
	}
}

func TestSwitcher_OpeningClearsGPrefix(t *testing.T) {
	t.Parallel()
	m, _ := switcherApp(t)
	m = pressKey(m, 'g')
	m, _ = step(m, ctrlO)
	if !m.switcher.isOpen || m.keyPrefix != "" {
		t.Fatalf("open=%v prefix=%q, want open with no prefix", m.switcher.isOpen, m.keyPrefix)
	}
}

func TestSwitcher_NavOpenSwitcherOpens(t *testing.T) {
	t.Parallel()
	m, _ := switcherApp(t)
	m, _ = step(m, nav.OpenSwitcher()())
	if !m.switcher.isOpen {
		t.Fatal("nav.OpenSwitcher did not open the switcher")
	}
}

func TestSwitcher_ClickOnTitleLeftGroupOpens(t *testing.T) {
	t.Parallel()
	for _, width := range []int{120, 12} {
		m, _ := switcherApp(t)
		m, _ = step(m, tea.WindowSizeMsg{Width: width, Height: 40})
		left := m.renderTitleRow(m.width).left
		m, _ = step(m, tea.MouseClickMsg{X: left.end - 1, Y: 0, Button: tea.MouseLeft})
		if !m.switcher.isOpen {
			t.Fatalf("width %d: click on %q did not open the switcher", width, titleOf(m))
		}
	}
}

func TestSwitcher_MouseWhileOpen(t *testing.T) {
	t.Parallel()
	m, stub := openedSwitcher(t)

	// Rows: header "git" at list line 0, then worktrees, log.
	logRow := ui.ChromeRows + switcherListTop + 2
	m, _ = step(m, tea.MouseWheelMsg{X: switcherX + 3, Y: logRow, Button: tea.MouseWheelDown})
	m, _ = step(m, tea.MouseClickMsg{X: switcherX + 3, Y: ui.ChromeRows + switcherListTop, Button: tea.MouseLeft})
	if !m.switcher.isOpen {
		t.Fatal("wheel or a click on a group header should not close the switcher")
	}
	next, _ := step(m, tea.MouseClickMsg{X: switcherX + 3, Y: logRow, Button: tea.MouseLeft})
	if next.switcher.isOpen || next.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("click on log row: open=%v tab=%q", next.switcher.isOpen, next.navState.ActiveTab())
	}
	next, _ = step(m, tea.MouseClickMsg{X: 100, Y: 30, Button: tea.MouseLeft})
	if next.switcher.isOpen || next.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("click outside: open=%v tab=%q", next.switcher.isOpen, next.navState.ActiveTab())
	}
	if len(stub.received) != 0 {
		t.Fatalf("mouse reached the page: %#v", stub.received)
	}
}

func TestSwitcher_Placement(t *testing.T) {
	t.Parallel()
	m, _ := openedSwitcher(t)

	frame := m.renderSwitcher()
	if w := lipgloss.Width(frame); w != switcherWidth {
		t.Fatalf("frame width = %d, want %d", w, switcherWidth)
	}
	lines := viewLines(m)
	top := lines[ui.ChromeRows]
	if !strings.HasPrefix(ansi.Cut(top, switcherX, m.width), "╭ go to ") {
		t.Fatalf("row %d = %q, want the frame top at column %d", ui.ChromeRows, top, switcherX)
	}
	if !strings.Contains(lines[ui.ChromeRows+1], "› ") {
		t.Fatalf("first body line = %q, want the query prompt", lines[ui.ChromeRows+1])
	}

	m, _ = step(m, tea.WindowSizeMsg{Width: 20, Height: 10})
	frame = m.renderSwitcher()
	if w := lipgloss.Width(frame); w != 16 {
		t.Fatalf("narrow frame width = %d, want 16", w)
	}
	if h := lipgloss.Height(frame); h != 10-ui.ChromeRows {
		t.Fatalf("short frame height = %d, want %d", h, 10-ui.ChromeRows)
	}
}

func TestSwitcher_AgentsBadgeIsActionableCount(t *testing.T) {
	t.Parallel()
	m, _ := switcherApp(t)
	m.stream.vm.Sessions = []server.SessionInfo{{ID: "a", Actionable: server.SessionActionable{Reason: "blocked"}}}
	m, _ = step(m, ctrlO)
	if got := m.switcher.picker.Items[7].Badge; got != "1" {
		t.Fatalf("agents badge = %q, want 1", got)
	}
}

func TestBindings_CtrlOSwitchesView(t *testing.T) {
	t.Parallel()
	ok := slices.ContainsFunc(Bindings(), func(b keys.Binding) bool {
		return slices.Equal(b.Seq, []string{"ctrl+o"}) && b.Title == "switch view" && slices.Contains(b.Categories, "App")
	})
	if !ok {
		t.Fatal("no App binding ctrl+o \"switch view\"")
	}
}

func TestSwitcher_Golden(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, query string }{
		{"switcher_empty", ""},
		{"switcher_query", "st"},
		{"switcher_no_matches", "zzz"},
	} {
		m, _ := switcherApp(t)
		m.stream.vm.Sessions = []server.SessionInfo{{ID: "a", Actionable: server.SessionActionable{Reason: "blocked"}}}
		m, _ = step(m, ctrlO)
		m = typeText(m, tc.query)
		assertGolden(t, tc.name, m.renderSwitcher()+"\n")
	}
}
