package app

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// lifecycleSpy records OnPageActivated / OnPageDeactivated call counts.
type lifecycleSpy struct {
	activated   int
	deactivated int
}

func (s *lifecycleSpy) Init() tea.Cmd                      { return nil }
func (s *lifecycleSpy) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *lifecycleSpy) View() tea.View                     { return tea.NewView("spy") }
func (s *lifecycleSpy) OnPageActivated() tea.Cmd {
	s.activated++
	return nil
}
func (s *lifecycleSpy) OnPageDeactivated() tea.Cmd {
	s.deactivated++
	return nil
}

func TestSwitchFiresDeactivateOnOldPage(t *testing.T) {
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

	m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())

	if spy.deactivated != 1 {
		t.Fatalf("expected OnPageDeactivated called once when pushing onto log, got %d", spy.deactivated)
	}
}

func TestBackFiresDeactivateOnPoppedAndActivateOnRevealed(t *testing.T) {
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

	// Push a commit page on top.
	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	m = updated.(Model)

	// Replace the pushed entry with a spy so we can observe OnPageDeactivated.
	poppedSpy := &lifecycleSpy{}
	m.stack[len(m.stack)-1] = historyEntry{
		viewState: m.stack[len(m.stack)-1].viewState,
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

func TestSwitchMsgChangesTabWithoutHistory(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected resize cmd on Switch")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.router.ActiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected empty stack after tab switch, got %d", len(m.stack))
	}
}

func TestShellChordDirectTabSwitchClearsHistory(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd on first g")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with gw")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected active tab worktrees, got %q", m.router.ActiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected empty stack after gw, got %d", len(m.stack))
	}
}

func TestShellChordSwitchesRelativeTabs(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g,")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected g, to move left to worktrees, got %q", m.router.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g.")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabLog {
		t.Fatalf("expected g. to move right to log, got %q", m.router.ActiveTab())
	}
}

func TestNumberKeysSwitchTabsGlobally(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching to worktrees with 1")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected 1 to switch to worktrees, got %q", m.router.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabLog {
		t.Fatalf("expected 2 to switch to log, got %q", m.router.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected 3 to switch to status, got %q", m.router.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected 4 to switch to commit, got %q", m.router.ActiveTab())
	}
}

func TestSwitchToUninitializedTabRunsInit(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	worktreesPage := m.livePageByTab[nav.TabWorktrees]
	if worktreesPage.didInit {
		t.Fatalf("expected cached worktrees tab to start uninitialized")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if cmd == nil {
		t.Fatalf("expected init/resize cmd on gw into uninitialized tab")
	}
	m = updated.(Model)
	if !m.livePageByTab[nav.TabWorktrees].didInit {
		t.Fatalf("expected gw to mark worktrees tab initialized")
	}
}

func TestOpenCommitAndBackRestoresTab(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when opening commit page")
	}
	m = updated.(Model)
	if len(m.stack) != 1 {
		t.Fatalf("expected stack depth 1, got %d", len(m.stack))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected active page commit, got %q", got)
	}
	if m.router.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected activeTab commit while commit is on stack, got %q", m.router.ActiveTab())
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.router.ActiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected empty stack after back, got %d", len(m.stack))
	}
}

func TestBackFromCommitRestoresSelectionInLog(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	testutil.WriteFile(t, repoDir, "a.txt", "one\n")
	testutil.CommitAll(t, repoDir, "base")
	testutil.WriteFile(t, repoDir, "a.txt", "two\n")
	testutil.CommitAll(t, repoDir, "middle")
	testutil.WriteFile(t, repoDir, "a.txt", "three\n")
	testutil.CommitAll(t, repoDir, "top")

	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD~1"})())
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: '.', Text: "."}) // move to newer commit (top)
	m = updated.(Model)
	top, err := git.LogEntries(repoDir, "HEAD", 1)
	if err != nil || len(top) == 0 {
		t.Fatalf("LogEntries: %v", err)
	}

	updated, _ = m.Update(nav.Back()())
	m = updated.(Model)
	logModel, ok := m.activePage().model.(logui.Model)
	if !ok {
		t.Fatalf("expected active model log.Model, got %T", m.activePage().model)
	}
	if got := logModel.SelectedRef(); got != top[0].FullHash {
		t.Fatalf("expected selected ref %q after back, got %q", top[0].FullHash, got)
	}
}

func TestOpenStatusAndBackRestoresLogTab(t *testing.T) {
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
	if len(m.stack) != 1 {
		t.Fatalf("expected stack depth 1, got %d", len(m.stack))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabStatus {
		t.Fatalf("expected active page status, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.router.ActiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected empty stack after back, got %d", len(m.stack))
	}
}

func TestSwitchAlwaysClearsStack(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	m = updated.(Model)
	if len(m.stack) != 1 {
		t.Fatalf("expected stack depth 1 after open, got %d", len(m.stack))
	}

	// Switch always clears the stack regardless of target tab or ViewContext.
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.router.ActiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected stack cleared after tab switch, got %d", len(m.stack))
	}
}

func TestCommitTabIsFirstClass(t *testing.T) {
	if got := navstate.ResolveTabID(nav.TabCommit); got != nav.TabCommit {
		t.Fatalf("expected commit to map to itself, got %q", got)
	}
}

func TestInitialCommitRouteUsesCommitTab(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"},
		ActiveWorktreePath: repoDir,
	})

	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected active page commit, got %q", got)
	}
	if m.router.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected active tab commit, got %q", m.router.ActiveTab())
	}
	if m.router.LiveTab() != nav.TabCommit {
		t.Fatalf("expected live tab commit, got %q", m.router.LiveTab())
	}
	if len(m.stack) != 0 {
		t.Fatalf("expected empty stack for initial commit view state (commit is a live tab), got %d", len(m.stack))
	}
}

func TestSwitchToCommitTabRestoresViewState(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	// Open commit from log.
	updated, _ := m.Update(nav.Open(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	m = updated.(Model)
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected commit page after Open, got %q", got)
	}

	// Switch away to status — clears stack, liveTab=status.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected status after pressing 3, got %q", m.router.ActiveTab())
	}

	// Switch to commit tab (4) — should restore remembered commit view state.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected commit tab after pressing 4, got %q", m.router.ActiveTab())
	}
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected commit page after switching to commit tab, got %q", got)
	}
}

func TestGotoCommitWithNoHistoryDefaultsToHEAD(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	// Switch directly to commit tab without any prior Open — should default to HEAD.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	if cmd == nil {
		t.Fatalf("expected cmd when switching to commit tab")
	}
	m = updated.(Model)
	if m.router.ActiveTab() != nav.TabCommit {
		t.Fatalf("expected commit tab, got %q", m.router.ActiveTab())
	}
	if got := m.activePage().viewState.Ref; got != "HEAD" {
		t.Fatalf("expected commit ref HEAD (default), got %q", got)
	}
}

func TestViewStateChangedUpdatesCommitTabState(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir, Ref: "HEAD"},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(nav.ViewStateChanged(nav.ViewState{
		Tab:          nav.TabCommit,
		WorktreeRoot: repoDir,
		Ref:          "HEAD~1",
	})())
	m = updated.(Model)

	if got := m.router.LastViewStateForTab(nav.TabCommit).Ref; got != "HEAD~1" {
		t.Fatalf("expected commit tab ref updated to HEAD~1, got %q", got)
	}

	// Switch to status then to commit — should restore the remembered ref.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)

	if got := m.activePage().viewState.Ref; got != "HEAD~1" {
		t.Fatalf("expected returning to commit to keep updated ref, got %q", got)
	}
}

func TestViewStateChangedPersistsForInactiveTabAndAppliesOnSwitch(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(nav.ViewStateChanged(nav.ViewState{
		Tab:          nav.TabCommit,
		WorktreeRoot: repoDir,
		Ref:          "HEAD~2",
	})())
	m = updated.(Model)

	if got := m.router.LastViewStateForTab(nav.TabCommit).Ref; got != "HEAD~2" {
		t.Fatalf("expected commit tab ref updated to HEAD~2, got %q", got)
	}

	// Switch to commit tab — should use the remembered ref.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)

	if got := m.activePage().viewState.Ref; got != "HEAD~2" {
		t.Fatalf("expected switch to commit to use updated ref, got %q", got)
	}
}

func TestBackWithEmptyHistoryQuits(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Back()())
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected model result")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit msg from back on root")
	}
}

func TestViewAppendsTabs(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	m.width = 120
	m.height = 24

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"worktrees", "log", "status", "commit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected tabs to include %q in %q", want, view)
		}
	}
}

func TestGChordOverlayIncludesAppAndChildHints(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	m.width = 100
	m.height = 24

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "prev tab") {
		t.Fatalf("expected app-level g chord hint in overlay")
	}
	if !strings.Contains(view, "go to top") {
		t.Fatalf("expected child-level g chord hint in overlay")
	}
}

func TestViewMergesTabsIntoFooterLine(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})
	m.width = 120
	m.height = 24

	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "worktrees") || !strings.Contains(last, "status") {
		t.Fatalf("expected tabs on footer line, got %q", last)
	}
	if strings.HasPrefix(strings.TrimLeft(last, " "), "? help") {
		t.Fatalf("expected tabs on left side of footer, got %q", last)
	}
}

func TestTabsUseBadgeCapsInFooter(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	m.width = 120
	m.height = 24

	last := strings.Split(m.View().Content, "\n")
	footer := last[len(last)-1]
	if !strings.Contains(footer, "") || !strings.Contains(footer, "") {
		t.Fatalf("expected pill badge caps in footer tabs")
	}
}

func TestViewMatchesScreenHeight(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	m.width = 80
	m.height = 24

	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected 24 lines, got %d", len(lines))
	}
}

func TestInjectTabsIntoFooterUsesEllipsisForRightTruncation(t *testing.T) {
	width := 32
	tabs := "worktrees log status"
	right := "· 󰉸 context: 1 · filetree · ? help"
	content := "body\n" + right

	merged := injectTabsIntoFooter(content, tabs, width)
	lines := strings.Split(ansi.Strip(merged), "\n")
	last := lines[len(lines)-1]

	if ansi.StringWidth(last) != width {
		t.Fatalf("expected merged footer width %d, got %d (%q)", width, ansi.StringWidth(last), last)
	}
	if !strings.Contains(last, "…") {
		t.Fatalf("expected right-side truncation to include ellipsis, got %q", last)
	}
}

func TestInjectTabsIntoFooterIgnoresRightLineLeadingPadding(t *testing.T) {
	width := 90
	tabs := " worktrees   log   status "
	right := strings.Repeat(" ", 120) + "· 󰉸 context: 1 · filetree · ? help"
	content := "body\n" + right

	merged := injectTabsIntoFooter(content, tabs, width)
	lines := strings.Split(ansi.Strip(merged), "\n")
	last := lines[len(lines)-1]

	if ansi.StringWidth(last) != width {
		t.Fatalf("expected merged footer width %d, got %d (%q)", width, ansi.StringWidth(last), last)
	}
	if !strings.Contains(last, "context: 1") {
		t.Fatalf("expected context label to remain visible after merge, got %q", last)
	}
}

func TestInjectTabsIntoFooterPreservesRightHintTailWithStatusPrefix(t *testing.T) {
	width := 90
	tabs := " worktrees   log   status "
	right := "staged README.md" + strings.Repeat(" ", 20) + "· 󰉸 context: 1 · filetree · ? help"
	content := "body\n" + right

	merged := injectTabsIntoFooter(content, tabs, width)
	lines := strings.Split(ansi.Strip(merged), "\n")
	last := lines[len(lines)-1]

	if ansi.StringWidth(last) != width {
		t.Fatalf("expected merged footer width %d, got %d (%q)", width, ansi.StringWidth(last), last)
	}
	if !strings.Contains(last, "context: 1") {
		t.Fatalf("expected context label to remain visible with status prefix, got %q", last)
	}
}
