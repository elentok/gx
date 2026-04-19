package worktrees

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestStatusBarViewShowsOnlyHelpPromptByDefault(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true

	line := ansi.Strip(m.statusBarView())
	if !strings.Contains(line, "? help") {
		t.Fatalf("expected footer to contain ? help, got %q", line)
	}
	for _, unwanted := range []string{"up", "down", "new worktree", "delete", "rename"} {
		if strings.Contains(line, unwanted) {
			t.Fatalf("expected compact footer without inline help, got %q", line)
		}
	}
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, "")
	m.ready = true
	m.width = 120
	m.height = 40
	m = m.resized()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if cmd != nil {
		t.Fatalf("expected no command when opening help")
	}
	m = updated.(Model)
	if m.mode != modeHelp {
		t.Fatalf("expected modeHelp after ?, got %v", m.mode)
	}
	if m.helpViewport.Width() == 0 || m.helpViewport.Height() == 0 {
		t.Fatalf("expected initialized help viewport, got %dx%d", m.helpViewport.Width(), m.helpViewport.Height())
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected no command when closing help")
	}
	m = updated.(Model)
	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after closing help, got %v", m.mode)
	}
}
