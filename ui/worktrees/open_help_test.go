package worktrees

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func newOpenHelpModel(t *testing.T) Model {
	t.Helper()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := New(*repo, "")
	m.ready = true
	m.width = 120
	m.height = 40
	return m.resized()
}

func TestOpenHelpOpensHelp(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)

	updated, _ := m.Update(nav.OpenHelp()())
	m = updated.(Model)
	if m.mode != modeHelp || !m.helpModel.IsOpen {
		t.Fatalf("expected help open, got mode %v", m.mode)
	}
}

func TestOpenHelpFromSearchTypesNothing(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if !m.InputFocused() {
		t.Fatal("expected search input focused after /")
	}

	updated, _ = m.Update(nav.OpenHelp()())
	m = updated.(Model)
	if m.mode != modeHelp || !m.helpModel.IsOpen {
		t.Fatalf("expected help open, got mode %v", m.mode)
	}
	if got := m.search.Query(); got != "" {
		t.Fatalf("search query = %q, want empty", got)
	}
}

func TestOpenHelpWithModalOpenDoesNothing(t *testing.T) {
	t.Parallel()
	m := newOpenHelpModel(t)
	m.confirm.IsOpen = true

	updated, _ := m.Update(nav.OpenHelp()())
	m = updated.(Model)
	if m.mode == modeHelp || m.helpModel.IsOpen {
		t.Fatal("expected help to stay closed while a modal is open")
	}

	m.confirm.IsOpen = false
	m.mode = modeRename
	updated, _ = m.Update(nav.OpenHelp()())
	m = updated.(Model)
	if m.mode != modeRename || m.helpModel.IsOpen {
		t.Fatalf("expected rename dialog kept, got mode %v", m.mode)
	}
}
