package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// lifecycleSpy records OnPageActivated / OnPageDeactivated call counts.
type lifecycleSpy struct {
	activated   int
	deactivated int
}

func (s *lifecycleSpy) Init() tea.Cmd                       { return nil }
func (s *lifecycleSpy) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *lifecycleSpy) View() tea.View                      { return tea.NewView("spy") }
func (s *lifecycleSpy) OnPageActivated() tea.Cmd {
	s.activated++
	return nil
}
func (s *lifecycleSpy) OnPageDeactivated() tea.Cmd {
	s.deactivated++
	return nil
}

func TestSwitchFiresDeactivateOnOldPage(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	spy := &lifecycleSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	m.livePageByTab[nav.TabLog] = live

	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())

	if spy.deactivated != 1 {
		t.Fatalf("expected OnPageDeactivated called once on outgoing page, got %d", spy.deactivated)
	}
}

func TestOpenFiresDeactivateOnOutgoingPage(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	spy := &lifecycleSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	m.livePageByTab[nav.TabLog] = live

	m.Update(nav.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())

	if spy.deactivated != 1 {
		t.Fatalf("expected OnPageDeactivated called once when pushing onto log, got %d", spy.deactivated)
	}
}

func TestBackFiresDeactivateOnPoppedAndActivateOnRevealed(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	// Replace the live log page model with a spy so we can observe OnPageActivated.
	revealedSpy := &lifecycleSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = revealedSpy
	m.livePageByTab[nav.TabLog] = live

	// Push a status page on top.
	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)

	// Replace the pushed entry with a spy so we can observe OnPageDeactivated.
	poppedSpy := &lifecycleSpy{}
	m.history[len(m.history)-1] = historyEntry{
		viewState: m.history[len(m.history)-1].viewState,
		model:     poppedSpy,
	}

	m.Update(nav.Back()())

	if poppedSpy.deactivated != 1 {
		t.Fatalf("expected OnPageDeactivated on popped page, got %d", poppedSpy.deactivated)
	}
	if revealedSpy.activated != 1 {
		t.Fatalf("expected OnPageActivated on revealed page, got %d", revealedSpy.activated)
	}
}
