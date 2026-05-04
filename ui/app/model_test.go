package app

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReplaceSwitchesTabWithoutHistory(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteWorktrees},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs")
	}
	m = updated.(Model)
	if m.activeTab != nav.RouteStatus {
		t.Fatalf("expected active tab status, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after tab replace, got %d", len(m.history))
	}
}

func TestShellChordReplacesTabWithoutHistory(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir},
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
	if m.activeTab != nav.RouteWorktrees {
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
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g,")
	}
	m = updated.(Model)
	if m.activeTab != nav.RouteWorktrees {
		t.Fatalf("expected g, to move left to worktrees, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching tabs with g.")
	}
	m = updated.(Model)
	if m.activeTab != nav.RouteLog {
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
		InitialRoute:       nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if cmd == nil {
		t.Fatalf("expected resize cmd when switching to worktrees with 1")
	}
	m = updated.(Model)
	if m.activeTab != nav.RouteWorktrees {
		t.Fatalf("expected 1 to switch to worktrees, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = updated.(Model)
	if m.activeTab != nav.RouteLog {
		t.Fatalf("expected 2 to switch to log, got %q", m.activeTab)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = updated.(Model)
	if m.activeTab != nav.RouteStatus {
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
		InitialRoute:       nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	worktreesPage := m.tabs[nav.RouteWorktrees]
	if worktreesPage.initialized {
		t.Fatalf("expected cached worktrees tab to start uninitialized")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if cmd == nil {
		t.Fatalf("expected init/resize cmd on gw into uninitialized tab")
	}
	m = updated.(Model)
	if !m.tabs[nav.RouteWorktrees].initialized {
		t.Fatalf("expected gw to mark worktrees tab initialized")
	}
}

func TestPushCommitAndBackRestoresTab(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when pushing commit route")
	}
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}
	if got := m.activePage().route.Kind; got != nav.RouteCommit {
		t.Fatalf("expected active page commit, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.activeTab != nav.RouteLog {
		t.Fatalf("expected active tab log after back, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after back, got %d", len(m.history))
	}
}

func TestPushStatusAndBackRestoresLogTab(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when pushing status route")
	}
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}
	if got := m.activePage().route.Kind; got != nav.RouteStatus {
		t.Fatalf("expected active page status, got %q", got)
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.activeTab != nav.RouteLog {
		t.Fatalf("expected active tab log after back, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after back, got %d", len(m.history))
	}
}

func TestReplaceClearsHistoryAfterPush(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})

	updated, _ := m.Update(nav.Push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: repoDir, Ref: "HEAD"})())
	m = updated.(Model)
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}

	updated, _ = m.Update(nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir})())
	m = updated.(Model)
	if m.activeTab != nav.RouteStatus {
		t.Fatalf("expected active tab status, got %q", m.activeTab)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected history cleared after tab replace, got %d", len(m.history))
	}
}

func TestCommitMapsToLogTab(t *testing.T) {
	if got := tabForRoute(nav.RouteCommit); got != nav.RouteLog {
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
		InitialRoute:       nav.Route{Kind: nav.RouteCommit, WorktreeRoot: repoDir, Ref: "HEAD"},
		ActiveWorktreePath: repoDir,
	})

	if got := m.activePage().route.Kind; got != nav.RouteCommit {
		t.Fatalf("expected active page commit, got %q", got)
	}
	if m.activeTab != nav.RouteLog {
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
		InitialRoute:       nav.Route{Kind: nav.RouteWorktrees},
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
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
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

func TestViewMergesTabsIntoFooterLine(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteWorktrees},
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
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
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
		InitialRoute:       nav.Route{Kind: nav.RouteLog, WorktreeRoot: repoDir},
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
