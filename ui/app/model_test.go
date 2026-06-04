package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
	stashlistui "github.com/elentok/gx/ui/stashlist"

	tea "charm.land/bubbletea/v2"
)

// inputFocusedStub is a page stub that reports InputFocused=true.
type inputFocusedStub struct{}

func (s *inputFocusedStub) Init() tea.Cmd                       { return nil }
func (s *inputFocusedStub) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *inputFocusedStub) View() tea.View                      { return tea.NewView("stub") }
func (s *inputFocusedStub) InputFocused() bool                  { return true }

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

func TestLogEnterRendersCommitDetailThroughAppShell(t *testing.T) {
	repoDir := testutil.TempRepoWithThreeCommits(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, initCmd := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = updated.(Model)
	m = runAppCmd(m, initCmd)

	m = runAppCmd(m, m.Init())

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Commit") {
		t.Fatalf("expected commit detail through app shell, got:\n%s", view)
	}
	if !strings.Contains(view, "c.txt") {
		t.Fatalf("expected commit file through app shell, got:\n%s", view)
	}
}

func runAppCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runAppCmd(m, c)
		}
		return m
	}
	next, _ := m.Update(msg)
	if m2, ok := next.(Model); ok {
		return m2
	}
	return m
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

	m.Update(nav.Open(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())

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
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after tab switch, got %d", len(m.history))
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
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected active tab worktrees, got %q", m.navState.ActiveTab())
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty stack after gw, got %d", len(m.history))
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
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected g, to move left to worktrees, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected g. to move right to log, got %q", m.navState.ActiveTab())
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
	if m.navState.ActiveTab() != nav.TabWorktrees {
		t.Fatalf("expected 1 to switch to worktrees, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabLog {
		t.Fatalf("expected 2 to switch to log, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected 3 to switch to status, got %q", m.navState.ActiveTab())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected 4 to switch to stash, got %q", m.navState.ActiveTab())
	}
}

func TestNumberKeysDoNotSwitchTabsWhenInputFocused(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	stub := &inputFocusedStub{}
	live := m.livePageByTab[nav.TabStatus]
	live.model = stub
	m.livePageByTab[nav.TabStatus] = live

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.navState.ActiveTab() != nav.TabStatus {
		t.Fatalf("expected tab to stay on status when input is focused, got %q", m.navState.ActiveTab())
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

func TestOpenStashAndBackRestoresTab(t *testing.T) {
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
	if got := navstate.ResolveTabID(nav.TabStash); got != nav.TabStash {
		t.Fatalf("expected stash to map to itself, got %q", got)
	}
}

func TestInitialStashRouteUsesStashTab(t *testing.T) {
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

func TestSwitchToStashTabRestoresTab(t *testing.T) {
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
	for _, want := range []string{"worktrees", "log", "status", "stash"} {
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

func newAppModel(t *testing.T) (Model, string) {
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
	return m, repoDir
}

func pressKey(m Model, key rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return updated.(Model)
}

func TestStashTabReachableVia4(t *testing.T) {
	m, _ := newAppModel(t)
	m = pressKey(m, '4')
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected stash tab via 4, got %q", m.navState.ActiveTab())
	}
}

func TestStashTabReachableViaGS(t *testing.T) {
	m, _ := newAppModel(t)
	m = pressKey(m, 'g')
	m = pressKey(m, 'S')
	if m.navState.ActiveTab() != nav.TabStash {
		t.Fatalf("expected stash tab via gS, got %q", m.navState.ActiveTab())
	}
}

func TestStashTabOpensInSplitState(t *testing.T) {
	m, repoDir := newAppModel(t)
	updated, _ := m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStash, WorktreeRoot: repoDir})())
	m = updated.(Model)
	// Give it a size so split layout is active.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = updated.(Model)

	page := m.livePageByTab[nav.TabStash]
	tab, ok := page.model.(stashlistui.Tab)
	if !ok {
		t.Fatalf("expected stash page to be stashlistui.Tab, got %T", page.model)
	}
	if !tab.IsSplit() {
		t.Fatal("expected stash tab to open in split state")
	}
}

func TestGCDoesNotSwitchToCommitTab(t *testing.T) {
	m, _ := newAppModel(t)
	before := m.navState.ActiveTab()
	m = pressKey(m, 'g')
	m = pressKey(m, 'c')
	// g+c should not change the active tab (no commit tab exists)
	if m.navState.ActiveTab() != before {
		t.Fatalf("g+c should not switch tabs, got %q", m.navState.ActiveTab())
	}
}

// reloadCounterModel wraps a real tea.Model, counting AutoReload calls while
// delegating to the real implementation.
type reloadCounterModel struct {
	inner   tea.Model
	reloads int
}

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
