package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

// inputFocusedStub is a page stub that reports InputFocused=true.
type inputFocusedStub struct{}

func (s *inputFocusedStub) Init() tea.Cmd                       { return nil }
func (s *inputFocusedStub) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *inputFocusedStub) View() tea.View                      { return tea.NewView("stub") }
func (s *inputFocusedStub) InputFocused() bool                  { return true }

// TestTicketsSearchDigitTypesIntoQueryNotTabSwitch covers ticket 14's fix:
// once "/" opens the Tickets tab's search box, a digit key must type into
// the query rather than being intercepted by the shell's digit-based
// tab-jump mnemonics (handleShellChordKey) before it ever reaches the tab.
func TestTicketsSearchDigitTypesIntoQueryNotTabSwitch(t *testing.T) {
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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(Model)

	if m.navState.ActiveTab() != nav.TabTickets {
		t.Fatalf("expected tab to stay on tickets when search input is active, got %q", m.navState.ActiveTab())
	}
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "Search") || !strings.Contains(content, "1") {
		t.Fatalf("expected the digit to be typed into the open search query, got:\n%s", content)
	}
}

func TestLogEnterRendersCommitDetailThroughAppShell(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(view, "Files") {
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

func TestBackWithEmptyHistoryQuits(t *testing.T) {
	t.Parallel()
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

func TestGChordOverlayIncludesAppAndChildHints(t *testing.T) {
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
	if !strings.Contains(view, "agents tab") {
		t.Fatalf("expected g a hint in overlay")
	}
}

func TestChordNotTriggeredWhileHelpFilterFocused(t *testing.T) {
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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(Model)

	// Open help, then activate its filter input.
	updated, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)

	// 'g' must type into the filter, not open the chord overlay.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)

	if m.keyPrefix != "" {
		t.Errorf("expected no chord prefix while help filter is focused, got %q", m.keyPrefix)
	}
}

func TestViewMatchesScreenHeight(t *testing.T) {
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
	m.width = 80
	m.height = 24

	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected 24 lines, got %d", len(lines))
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

// modalOpenStub is a page stub that reports ModalOpen=true.
type modalOpenStub struct{}

func (s *modalOpenStub) Init() tea.Cmd                       { return nil }
func (s *modalOpenStub) Update(tea.Msg) (tea.Model, tea.Cmd) { return s, nil }
func (s *modalOpenStub) View() tea.View                      { return tea.NewView("stub") }
func (s *modalOpenStub) ModalOpen() bool                     { return true }
