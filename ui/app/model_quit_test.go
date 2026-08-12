package app

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// findButtonCoords locates the on-screen (x, y) of the given quit-confirm
// button text (" Yes " or " No ") using the same centering math
// ui.OverlayCenter applies in Model.View, so the click lands where the
// button is actually rendered rather than a hardcoded guess.
func findButtonCoords(t *testing.T, m Model, button string) (x, y int) {
	t.Helper()
	view := m.quitConfirm.View(m.width)
	fgW := lipgloss.Width(view)
	fgH := lipgloss.Height(view)
	ox := max((m.width-fgW)/2, 0)
	oy := max((m.height-fgH)/2, 0)
	for row, line := range strings.Split(view, "\n") {
		plain := ansi.Strip(line)
		if before, _, found := strings.Cut(plain, button); found {
			col := ansi.StringWidth(before)
			return ox + col + ansi.StringWidth(button)/2, oy + row
		}
	}
	t.Fatalf("could not find button %q in rendered quit-confirm modal:\n%s", button, ansi.Strip(view))
	return 0, 0
}

// quitBlockingStub is a page stub whose CanQuit() reports a loop in progress.
type quitBlockingStub struct{ canQuit bool }

func (s *quitBlockingStub) Init() tea.Cmd                       { return nil }
func (s *quitBlockingStub) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *quitBlockingStub) View() tea.View                      { return tea.NewView("stub") }
func (s *quitBlockingStub) CanQuit() bool                       { return s.canQuit }

func newAppWithQuitGuard(t *testing.T, canQuit bool) Model {
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
	m.ensureLivePages()
	m.livePageByTab[nav.TabTickets] = livePage{model: &quitBlockingStub{canQuit: canQuit}}
	return m
}

func TestBackWithLoopRunningShowsConfirmInsteadOfQuitting(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)

	updated, cmd := m.Update(nav.Back()())
	m = updated.(Model)

	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatalf("expected no quit msg while loop is running")
			}
		}
	}
}

func TestQuitConfirmCancelReturnsToAppWithLoopUnaffected(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)

	updated, _ := m.Update(nav.Back()())
	m = updated.(Model)
	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be closed after cancel")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatalf("expected no quit msg after canceling")
			}
		}
	}
	stub := m.livePageByTab[nav.TabTickets].model.(*quitBlockingStub)
	if stub.canQuit {
		t.Fatalf("expected the loop-running stub state to be unaffected by cancel")
	}
}

func TestQuitConfirmAcceptQuits(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)

	updated, _ := m.Update(nav.Back()())
	m = updated.(Model)
	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be closed after confirming")
	}
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit msg after confirming")
	}
}

func TestClickQuitConfirmYesQuits(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(nav.Back()())
	m = updated.(Model)
	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}

	x, y := findButtonCoords(t, m, " Yes ")
	updated, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be closed after clicking Yes")
	}
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit msg after clicking Yes")
	}
}

func TestClickQuitConfirmNoCancels(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(nav.Back()())
	m = updated.(Model)
	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}

	x, y := findButtonCoords(t, m, " No ")
	updated, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be closed after clicking No")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatalf("expected no quit msg after clicking No")
			}
		}
	}
}

func TestForceQuitWithLoopRunningShowsConfirmInsteadOfQuitting(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, false)

	updated, cmd := m.Update(nav.ForceQuit()())
	m = updated.(Model)

	if !m.quitConfirm.IsOpen {
		t.Fatalf("expected quit confirm dialog to be open")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatalf("expected no quit msg while loop is running")
			}
		}
	}
}

func TestForceQuitWithNoLoopRunningQuitsImmediately(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, true)

	updated, cmd := m.Update(nav.ForceQuit()())
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected no confirm dialog when no loop is running")
	}
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit msg from ctrl+c when no loop is running")
	}
}

func TestBackWithNoLoopRunningQuitsImmediately(t *testing.T) {
	t.Parallel()
	m := newAppWithQuitGuard(t, true)

	updated, cmd := m.Update(nav.Back()())
	m = updated.(Model)

	if m.quitConfirm.IsOpen {
		t.Fatalf("expected no confirm dialog when no loop is running")
	}
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit msg from back on root when no loop is running")
	}
}
