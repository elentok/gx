package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
	stashlistui "github.com/elentok/gx/ui/stashlist"

	tea "charm.land/bubbletea/v2"
)

func TestOpenStashAndBackRestoresTab(t *testing.T) {
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

	updated, cmd := m.Update(nav.Open(nav.ViewState{Tab: nav.TabStash, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when opening stash page")
	}
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected stack depth 1, got %d", len(m.history))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabStash {
		t.Fatalf("expected active page stash, got %q", got)
	}
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected activeTab stash while stash is on stack, got %q", m.navState.ActiveTab())
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after back, got %d", len(m.history))
	}
}

func TestOpenStatusAndBackRestoresLogTab(t *testing.T) {
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

	updated, cmd := m.Update(nav.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when opening status page")
	}
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected stack depth 1, got %d", len(m.history))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabStatus {
		t.Fatalf("expected active page status, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after back, got %d", len(m.history))
	}
}

func TestSwitchAlwaysClearsStack(t *testing.T) {
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

	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabStash, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected stack depth 1 after open, got %d", len(m.history))
	}

	// Switch always clears the stack regardless of target tab or ViewContext.
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected stack cleared after tab switch, got %d", len(m.history))
	}
}

func TestStashTabIsFirstClass(t *testing.T) {
	t.Parallel()
	if got := navstate.ResolveTabID(nav.TabStash); got != nav.TabStash {
		t.Fatalf("expected stash to map to itself, got %q", got)
	}
}

func TestInitialStashRouteUsesStashTab(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStash, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	if got := m.activePage().viewState.Tab; got != nav.TabStash {
		t.Fatalf("expected active page stash, got %q", got)
	}
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected active tab stash, got %q", m.navState.ActiveTab())
	}
	if m.navState.LiveTab() != nav.TabStash {
		t.Fatalf("expected live tab stash, got %q", m.navState.LiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack for initial stash view state, got %d", len(m.history))
	}
}

func TestPRsTabIsFirstClass(t *testing.T) {
	t.Parallel()
	if got := navstate.ResolveTabID(nav.TabPRs); got != nav.TabPRs {
		t.Fatalf("expected prs to map to itself, got %q", got)
	}
}

func TestInitialPRsRouteUsesPRsTab(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabPRs, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	if got := m.activePage().viewState.Tab; got != nav.TabPRs {
		t.Fatalf("expected active page prs, got %q", got)
	}
	if m.navState.ActiveTab() != nav.TabPRs {
		t.Fatalf("expected active tab prs, got %q", m.navState.ActiveTab())
	}
	if m.navState.LiveTab() != nav.TabPRs {
		t.Fatalf("expected live tab prs, got %q", m.navState.LiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack for initial prs view state, got %d", len(m.history))
	}
}

func TestTicketsTabIsFirstClass(t *testing.T) {
	t.Parallel()
	if got := navstate.ResolveTabID(nav.TabTickets); got != nav.TabTickets {
		t.Fatalf("expected tickets to map to itself, got %q", got)
	}
}

func TestInitialTicketsRouteUsesTicketsTab(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	if got := m.activePage().viewState.Tab; got != nav.TabTickets {
		t.Fatalf("expected active page tickets, got %q", got)
	}
	if m.navState.ActiveTab() != nav.TabTickets {
		t.Fatalf("expected active tab tickets, got %q", m.navState.ActiveTab())
	}
	if m.navState.LiveTab() != nav.TabTickets {
		t.Fatalf("expected live tab tickets, got %q", m.navState.LiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack for initial tickets view state, got %d", len(m.history))
	}
}

func TestSwitchToStashTabRestoresTab(t *testing.T) {
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

	// Switch away to status, then press 4 to go to stash.
	updated, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected status after pressing 3, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected stash tab after pressing 4, got %q", m.navState.ActiveTab())
	}
	if got := m.activePage().viewState.Tab; got != nav.TabStash {
		t.Fatalf("expected stash page after switching to stash tab, got %q", got)
	}
}

func TestSwitchFromLogToStatusCarriesWorktree(t *testing.T) {
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

	// Switch to status with no explicit worktree — should inherit log's worktree.
	updated, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if got := m.activePage().viewState.WorktreeRoot; got != repoDir {
		t.Fatalf("expected status WorktreeRoot %q, got %q", repoDir, got)
	}
}

func TestSwitchFromStatusToLogCarriesWorktree(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if got := m.activePage().viewState.WorktreeRoot; got != repoDir {
		t.Fatalf("expected log WorktreeRoot %q, got %q", repoDir, got)
	}
}

func TestStashTabReachableVia4(t *testing.T) {
	t.Parallel()
	m, _ := newAppModel(t)
	m = pressKey(m, '4')
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected stash tab via 4, got %q", m.navState.ActiveTab())
	}
}

func TestStashTabReachableViaGS(t *testing.T) {
	t.Parallel()
	m, _ := newAppModel(t)
	m = pressKey(m, 'g')
	m = pressKey(m, 'S')
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected stash tab via gS, got %q", m.navState.ActiveTab())
	}
}

func TestStashTabOpensInSplitState(t *testing.T) {
	t.Parallel()
	m, repoDir := newAppModel(t)
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStash, WorktreeRoot: repoDir})())
	m = updated.(Model)
	// Give it a size so split layout is active.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = updated.(Model)

	page := m.livePageByTab[nav.TabStash]
	tab, ok := page.model.(stashlistui.Model)
	if !ok {
		t.Fatalf("expected stash page to be stashlistui.Model, got %T", page.model)
	}
	if !tab.IsSplit() {
		t.Fatal("expected stash tab to open in split state")
	}
}

func TestGCDoesNotSwitchToCommitTab(t *testing.T) {
	t.Parallel()
	m, _ := newAppModel(t)
	before := m.navState.ActiveTab()
	m = pressKey(m, 'g')
	m = pressKey(m, 'c')
	// g+c should not change the active tab (no commit tab exists)
	if m.navState.ActiveTab() != before {
		t.Fatalf("g+c should not switch tabs, got %q", m.navState.ActiveTab())
	}
}
