package app

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

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
	if m.activeTab != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after tab switch, got %d", len(m.history))
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
	if m.activeTab != nav.TabWorktrees {
		t.Fatalf("expected active tab worktrees, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after gw, got %d", len(m.history))
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
	if m.activeTab != nav.TabWorktrees {
		t.Fatalf("expected g, to move left to worktrees, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g.")
	}
	m = updated.(Model)
	if m.activeTab != nav.TabLog {
		t.Fatalf("expected g. to move right to log, got %q", m.activeTab)
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
	if m.activeTab != nav.TabWorktrees {
		t.Fatalf("expected 1 to switch to worktrees, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.activeTab != nav.TabLog {
		t.Fatalf("expected 2 to switch to log, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.activeTab != nav.TabStatus {
		t.Fatalf("expected 3 to switch to status, got %q", m.activeTab)
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
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
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected active page commit, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.activeTab != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after back, got %d", len(m.history))
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
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}
	if got := m.activePage().viewState.Tab; got != nav.TabStatus {
		t.Fatalf("expected active page status, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.activeTab != nav.TabLog {
		t.Fatalf("expected active tab log after back, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after back, got %d", len(m.history))
	}
}

func TestSwitchClearsHistoryAfterOpen(t *testing.T) {
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
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}

	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if m.activeTab != nav.TabStatus {
		t.Fatalf("expected active tab status, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected history cleared after tab switch, got %d", len(m.history))
	}
}

func TestSameViewContextDifferentOptionsPreservesHistory(t *testing.T) {
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
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1 after open, got %d", len(m.history))
	}

	// Switch to same tab with same ViewContext but different ViewOptions (FocusSubject).
	// ViewOptions changes must NOT trigger page reconstruction, so history is preserved.
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir, FocusSubject: "some.go"})())
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected history preserved when ViewContext unchanged, got %d", len(m.history))
	}
}

func TestDifferentViewContextClearsHistory(t *testing.T) {
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
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1 after open, got %d", len(m.history))
	}

	// Switch to same tab with a different ViewContext (Ref changed).
	// ViewContext changes MUST trigger page reconstruction, so history is cleared.
	updated, _ = m.Update(nav.Switch(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: repoDir, Ref: "HEAD~1"})())
	m = updated.(Model)
	if len(m.history) != 0 {
		t.Fatalf("expected history cleared when ViewContext changed, got %d", len(m.history))
	}
}

func TestTabSwitchRestoresCommitRouteInLogTab(t *testing.T) {
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
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected commit page after Open, got %q", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if got := m.activePage().viewState.Tab; got != nav.TabStatus {
		t.Fatalf("expected status page after switching tab, got %q", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if got := m.activePage().viewState.Tab; got != nav.TabCommit {
		t.Fatalf("expected returning to log tab to restore commit page, got %q", got)
	}
}

func TestViewStateChangedUpdatesActiveTabViewState(t *testing.T) {
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

	if got := m.lastViewStateByTab[nav.TabLog].Ref; got != "HEAD~1" {
		t.Fatalf("expected log tab ref updated to HEAD~1, got %q", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)

	if got := m.activePage().viewState.Ref; got != "HEAD~1" {
		t.Fatalf("expected returning to log to keep updated ref, got %q", got)
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

	if got := m.lastViewStateByTab[nav.TabLog].Ref; got != "HEAD~2" {
		t.Fatalf("expected inactive log tab ref updated to HEAD~2, got %q", got)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)

	if got := m.activePage().viewState.Ref; got != "HEAD~2" {
		t.Fatalf("expected switch to log to use updated ref, got %q", got)
	}
}

func TestCommitMapsToLogTab(t *testing.T) {
	if got := tabForRoute(nav.TabCommit); got != nav.TabLog {
		t.Fatalf("expected commit to map to log tab, got %q", got)
	}
}

func TestInitialCommitRouteStartsOnCommitPage(t *testing.T) {
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
	if m.activeTab != nav.TabLog {
		t.Fatalf("expected active tab log for commit-backed page, got %q", m.activeTab)
	}
	if len(m.history) != 1 {
		t.Fatalf("expected initial commit route in history, got %d", len(m.history))
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
	m.width = 80
	m.height = 24

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"worktrees", "log", "status"} {
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
	m.width = 80
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
	m.width = 80
	m.height = 24

	last := strings.Split(m.View().Content, "\n")
	footer := last[len(last)-1]
	if !strings.Contains(footer, "\uE0B6") || !strings.Contains(footer, "\uE0B4") {
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
	tabs := " worktrees   log   status "
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
	tabs := " worktrees   log   status "
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
