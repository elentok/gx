package app

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// mouseRecorderStub records every msg it receives, so tests can assert a
// mouse click that misses the tab bar still reaches the active page.
type mouseRecorderStub struct{ received []tea.Msg }

func (s *mouseRecorderStub) Init() tea.Cmd { return nil }
func (s *mouseRecorderStub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.received = append(s.received, msg)
	return s, nil
}
func (s *mouseRecorderStub) View() tea.View { return tea.NewView("stub") }

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

// tabClickX returns an X coordinate inside the rendered label for the given
// tab at the app's current width, using the same layout tabHitAt hit-tests
// against, so the test doesn't hardcode column numbers that would drift the
// moment a label or badge padding changes.
func tabClickX(t *testing.T, m Model, id nav.TabID) int {
	t.Helper()
	pos := 0
	for _, spec := range m.tabSpecs() {
		w := ansi.StringWidth(renderTab(spec))
		if spec.id == id {
			return pos + w/2
		}
		pos += w + 1
	}
	t.Fatalf("tab %q not found in tabSpecs", id)
	return 0
}

func TestClickTabLabelSwitchesActiveTab(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)
	x := tabClickX(t, m, nav.TabLog)

	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: m.height - 1, Button: tea.MouseLeft})
	m = updated.(Model)

	if got := m.navState.ActiveTab(); got != nav.TabLog {
		t.Fatalf("active tab = %q, want %q", got, nav.TabLog)
	}
}

func TestClickOutsideTabBarFallsThroughToActivePage(t *testing.T) {
	t.Parallel()
	m := newAppForMouseTest(t)
	stub := &mouseRecorderStub{}
	live := m.livePageByTab[nav.TabWorktrees]
	live.model = stub
	m.livePageByTab[nav.TabWorktrees] = live

	click := tea.MouseClickMsg{X: m.width - 1, Y: 0, Button: tea.MouseLeft}
	updated, _ := m.Update(click)
	m = updated.(Model)

	if got := m.navState.ActiveTab(); got != nav.TabWorktrees {
		t.Fatalf("active tab changed to %q, want unchanged %q", got, nav.TabWorktrees)
	}
	if len(stub.received) != 1 || stub.received[0] != click {
		t.Fatalf("expected the click to reach the active page, got %#v", stub.received)
	}
}
