package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// autoReloadSpy is a page stub that counts AutoReload calls.
type autoReloadSpy struct {
	reloads int
}

func (s *autoReloadSpy) Init() tea.Cmd                       { return nil }
func (s *autoReloadSpy) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *autoReloadSpy) View() tea.View                      { return tea.NewView("spy") }
func (s *autoReloadSpy) AutoReload() tea.Cmd {
	s.reloads++
	return nil
}

func TestGate_BareSwitch_FreshTab_NoAutoReload(t *testing.T) {
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

	spy := &autoReloadSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// Switch away and back with no mutations — log is fresh.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir})())

	if spy.reloads != 0 {
		t.Errorf("expected 0 AutoReload calls on fresh tab switch, got %d", spy.reloads)
	}
}

func TestGate_BareSwitch_StaleTab_OneAutoReload(t *testing.T) {
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

	spy := &autoReloadSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// Switch to status first so log is a background tab.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)

	// Emit RepoMutated while on status — log becomes stale.
	updated, _ = m.Update(nav.RepoMutated()())
	m = updated.(Model)

	// Switch back to log — should auto-reload exactly once.
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir})())

	if spy.reloads != 1 {
		t.Errorf("expected exactly 1 AutoReload on stale tab switch, got %d", spy.reloads)
	}
}

func TestGate_FocusSubject_ForcesReloadWhenFresh(t *testing.T) {
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

	spy := &autoReloadSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// Switch to status and back with a FocusSubject — no mutations, but reload forced.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir, FocusSubject: "abc"})())

	if spy.reloads != 1 {
		t.Errorf("expected 1 AutoReload for FocusSubject switch, got %d", spy.reloads)
	}
}

func TestGate_ReconstructPath_NoDoubleReload(t *testing.T) {
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

	// Switch log to a different worktree root — triggers reconstruct, MarkLoaded is called.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir + "/other"})())
	m = updated.(Model)

	// Now switch to status and back to log with no mutations: should not auto-reload.
	spy := &autoReloadSpy{}
	live := m.livePageByTab[nav.TabLog]
	live.model = spy
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir + "/other"})())

	if spy.reloads != 0 {
		t.Errorf("expected 0 AutoReload after reconstruct (Init already loaded), got %d", spy.reloads)
	}
}

// reloadCounterModel wraps a real tea.Model, counting AutoReload calls while
// delegating to the real implementation.
type reloadCounterModel struct {
	inner     tea.Model
	reloads   int
	needsLoad bool
}

func (c *reloadCounterModel) NeedsInitialLoad() bool { return c.needsLoad }

func (c *reloadCounterModel) Init() tea.Cmd {
	return c.inner.Init()
}

func (c *reloadCounterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := c.inner.Update(msg)
	c.inner = updated
	return c, cmd
}

func (c *reloadCounterModel) View() tea.View {
	return c.inner.View()
}

func (c *reloadCounterModel) AutoReload() tea.Cmd {
	c.reloads++
	if r, ok := c.inner.(pageAutoReloadable); ok {
		return r.AutoReload()
	}
	return nil
}

// TestE2E_LogStatusLog_NoReloadWhenFresh is the regression gate for the
// epoch-based tab-reload feature: log → status → log with no repo mutations
// must not trigger a git re-fetch on the real log model.
func TestE2E_LogStatusLog_NoReloadWhenFresh(t *testing.T) {
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

	counter := &reloadCounterModel{inner: m.livePageByTab[nav.TabLog].model}
	live := m.livePageByTab[nav.TabLog]
	live.model = counter
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// log → status (no mutation)
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)

	// status → log: gate is fresh, real log model must not AutoReload
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir})())

	if counter.reloads != 0 {
		t.Errorf("log→status→log with no mutations: expected 0 AutoReload calls on real log model, got %d", counter.reloads)
	}
}

// TestE2E_StatusCommit_LogReloadsOnSwitch verifies that after a commit in status
// emits nav.RepoMutated, the log tab auto-reloads exactly once when activated.
func TestE2E_StatusCommit_LogReloadsOnSwitch(t *testing.T) {
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

	counter := &reloadCounterModel{inner: m.livePageByTab[nav.TabLog].model}
	live := m.livePageByTab[nav.TabLog]
	live.model = counter
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// Switch to status.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)

	// Simulate a commit completing in status (emits RepoMutated).
	updated, _ = m.Update(nav.RepoMutated()())
	m = updated.(Model)

	// Switch back to log: gate is stale, real log model must AutoReload exactly once.
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir})())

	if counter.reloads != 1 {
		t.Errorf("status commit → log: expected 1 AutoReload call, got %d", counter.reloads)
	}
}

// TestE2E_LogReportsRefThenSwitchReusesPage guards against the cache-drift bug:
// the log page is stamped with an empty Ref on first switch, but the model
// reports its normalized Ref ("HEAD") via ViewStateChanged. If the live page's
// stamp isn't kept in sync, the next switch sees a context mismatch and rebuilds
// the page from scratch (losing cached rows). The page model must survive a
// log → status → log round-trip.
func TestE2E_LogReportsRefThenSwitchReusesPage(t *testing.T) {
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

	counter := &reloadCounterModel{inner: m.livePageByTab[nav.TabLog].model}
	live := m.livePageByTab[nav.TabLog]
	live.model = counter
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// The log model reports its normalized ref once loaded.
	updated, _ := m.Update(nav.ViewStateChanged(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir, Ref: "HEAD"})())
	m = updated.(Model)

	// log → status → log with no mutations. Switch with a bare tab (no worktree/ref),
	// matching the real key dispatch, so the destination is resolved from per-tab memory.
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus})())
	m = updated.(Model)
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog})())
	m = updated.(Model)

	if got := m.livePageByTab[nav.TabLog].model; got != tea.Model(counter) {
		t.Errorf("log page was rebuilt across switch; expected cached model to be reused")
	}
}

// TestE2E_InterruptedInitialLoad_ReloadsOnReturn guards against a page getting
// stuck on its loading state: if the user leaves before the Init reload lands,
// the gate was optimistically stamped at switch-in, so a plain revisit would
// not reload. A page reporting NeedsInitialLoad must trigger an AutoReload when
// re-activated.
func TestE2E_InterruptedInitialLoad_ReloadsOnReturn(t *testing.T) {
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

	// Simulate the log page whose initial load never completed (rows still nil).
	counter := &reloadCounterModel{inner: m.livePageByTab[nav.TabLog].model, needsLoad: true}
	live := m.livePageByTab[nav.TabLog]
	live.model = counter
	live.didInit = true
	m.livePageByTab[nav.TabLog] = live

	// log → status → log with no mutations: the unloaded page must reload.
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus})())
	m = updated.(Model)
	m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog})())

	if counter.reloads != 1 {
		t.Errorf("interrupted initial load → return: expected 1 AutoReload, got %d", counter.reloads)
	}
}
