package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

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

func TestForceQuitWithLoopRunningShowsConfirmInsteadOfQuitting(t *testing.T) {
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
