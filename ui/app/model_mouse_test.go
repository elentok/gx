package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// mouseRecorderStub records every msg it receives and draws a fixed body, so
// tests can assert what reaches the page and where the shell puts it.
type mouseRecorderStub struct {
	received []tea.Msg
	cursor   *tea.Cursor
}

func (s *mouseRecorderStub) Init() tea.Cmd { return nil }
func (s *mouseRecorderStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}
func (s *mouseRecorderStub) View() tea.View {
	v := tea.NewView("stub")
	v.Cursor = s.cursor
	return v
}

func newAppForMouseTest(t *testing.T) Model {
	t.Helper()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m = runAppCmd(m, cmd)
	m = runAppCmd(m, m.Init())
	return m
}

// withStubPage puts a recorder in place of the worktrees page.
func withStubPage(m Model) (Model, *mouseRecorderStub) {
	stub := &mouseRecorderStub{}
	live := m.livePageByTab[nav.TabWorktrees]
	live.model = stub
	m.livePageByTab[nav.TabWorktrees] = live
	return m, stub
}

func viewLines(m Model) []string {
	return strings.Split(ansi.Strip(m.View().Content), "\n")
}

func TestViewDrawsTitleRowAndBlankRowAbovePage(t *testing.T) {
	t.Parallel()
	m, _ := withStubPage(newAppForMouseTest(t))

	lines := viewLines(m)
	if len(lines) != m.height {
		t.Fatalf("frame has %d lines, want %d", len(lines), m.height)
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "gx › worktrees ▾") || !strings.HasSuffix(strings.TrimSpace(lines[0]), "? help") {
		t.Errorf("title row = %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "" {
		t.Errorf("row 1 = %q, want blank", lines[1])
	}
	if strings.TrimSpace(lines[2]) != "stub" {
		t.Errorf("row 2 = %q, want the page's first row", lines[2])
	}
	if last := lines[len(lines)-1]; strings.TrimSpace(last) != "" {
		t.Errorf("last row = %q, want no tab bar", last)
	}
}

func TestTitleRowSpansCoverLeftGroupAndHelp(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)

	row := m.renderTitleRow(m.width)
	plain := ansi.Strip(row.line)
	if got := ansi.Cut(plain, row.left.start, row.left.end); got != "gx › worktrees ▾" {
		t.Errorf("left span = %q", got)
	}
	if got := ansi.Cut(plain, row.help.start, row.help.end); got != "? help" {
		t.Errorf("help span = %q", got)
	}
	if w := ansi.StringWidth(plain); w != m.width {
		t.Errorf("title row width = %d, want %d", w, m.width)
	}
}

func TestPageReceivesHeightMinusChrome(t *testing.T) {
	t.Parallel()
	m, stub := withStubPage(newAppForMouseTest(t))

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	want := tea.WindowSizeMsg{Width: 100, Height: 28}
	if len(stub.received) != 1 || stub.received[0] != want {
		t.Fatalf("page received %#v, want %#v", stub.received, want)
	}
	if got := len(viewLines(m)); got != 30 {
		t.Fatalf("frame has %d lines, want 30", got)
	}
}

func TestMouseOnPageRowsReachesPageShiftedUp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		msg  tea.MouseMsg
		want tea.Msg
	}{
		{"click", tea.MouseClickMsg{X: 7, Y: 5, Button: tea.MouseLeft}, tea.MouseClickMsg{X: 7, Y: 3, Button: tea.MouseLeft}},
		{"release", tea.MouseReleaseMsg{X: 7, Y: 2, Button: tea.MouseLeft}, tea.MouseReleaseMsg{X: 7, Y: 0, Button: tea.MouseLeft}},
		{"motion", tea.MouseMotionMsg{X: 7, Y: 10}, tea.MouseMotionMsg{X: 7, Y: 8}},
		{"wheel", tea.MouseWheelMsg{X: 7, Y: 39, Button: tea.MouseWheelDown}, tea.MouseWheelMsg{X: 7, Y: 37, Button: tea.MouseWheelDown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, stub := withStubPage(newAppForMouseTest(t))

			m.Update(tc.msg)

			if len(stub.received) != 1 || stub.received[0] != tc.want {
				t.Fatalf("page received %#v, want %#v", stub.received, tc.want)
			}
		})
	}
}

func TestMouseOnChromeRowsStaysWithApp(t *testing.T) {
	t.Parallel()
	m, stub := withStubPage(newAppForMouseTest(t))

	for _, msg := range []tea.MouseMsg{
		tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft},
		tea.MouseClickMsg{X: m.width - 2, Y: 0, Button: tea.MouseLeft},
		tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft},
		tea.MouseWheelMsg{X: 3, Y: 1, Button: tea.MouseWheelDown},
		tea.MouseMotionMsg{X: 3, Y: 0},
	} {
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}

	if len(stub.received) != 0 {
		t.Fatalf("page received %#v, want nothing", stub.received)
	}
	if got := m.navState.ActiveTab(); got != nav.TabWorktrees {
		t.Fatalf("active tab = %q, want unchanged", got)
	}
}

func TestPageCursorMovesBelowChrome(t *testing.T) {
	t.Parallel()
	m, stub := withStubPage(newAppForMouseTest(t))
	stub.cursor = tea.NewCursor(4, 6)

	got := m.View().Cursor
	if got == nil || got.X != 4 || got.Y != 8 {
		t.Fatalf("cursor = %+v, want (4, 8)", got)
	}
	if stub.cursor.Y != 6 {
		t.Fatalf("page cursor mutated to Y=%d", stub.cursor.Y)
	}
}

func TestNotificationsStartBelowChrome(t *testing.T) {
	t.Parallel()
	m, _ := withStubPage(newAppForMouseTest(t))

	updated, _ := m.Update(notify.Info("hello there")())
	m = updated.(Model)

	lines := viewLines(m)
	for _, line := range lines[:ui.ChromeRows] {
		if strings.Contains(line, "hello there") {
			t.Fatalf("notification covers the chrome rows: %q", line)
		}
	}
	if !strings.Contains(strings.Join(lines[ui.ChromeRows:], "\n"), "hello there") {
		t.Fatal("notification not drawn")
	}
}

func titleOf(m Model) string {
	return strings.TrimSpace(viewLines(m)[0])
}

func TestTitleFollowsTabSwitches(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)

	steps := []struct {
		keys string
		want string
	}{
		{"2", "log"},
		{"gs", "status"},
		{"g,", "log"},
		{"g.", "status"},
		{"8", "agents"},
		{"gw", "worktrees"},
	}
	for _, step := range steps {
		for _, r := range step.keys {
			m = pressKey(m, r)
		}
		if got := titleOf(m); !strings.HasPrefix(got, "gx › "+step.want+" ▾") {
			t.Fatalf("after %q title = %q, want view %q", step.keys, got, step.want)
		}
	}
}

func TestTitleNamesDrillDownPage(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)
	m = pressKey(m, '3')

	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: m.settings.ActiveWorktreePath})())
	m = updated.(Model)

	if got := titleOf(m); !strings.HasPrefix(got, "gx › log ▾") {
		t.Fatalf("title = %q, want the drill-down view", got)
	}
}
