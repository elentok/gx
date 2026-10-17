package worktrees

import (
	"slices"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	keymgr "github.com/elentok/gx/ui/keys"

	tea "charm.land/bubbletea/v2"
)

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	t.Parallel()
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
	if m.helpModel.Viewport.Width() == 0 || m.helpModel.Viewport.Height() == 0 {
		t.Fatalf("expected initialized help viewport, got %dx%d", m.helpModel.Viewport.Width(), m.helpModel.Viewport.Height())
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

func TestHelpListsExtraAppBindings(t *testing.T) {
	t.Parallel()
	app := keymgr.New([]keymgr.Binding{
		{ID: "app-switch-view", Seq: []string{"ctrl+o"}, Categories: []string{"App"}, Title: "switch view"},
	})
	m := NewWithSettings(git.Repo{}, "", ui.Settings{}, app)

	for _, s := range m.helpModel.KeySections {
		if s.Title == "App" && slices.ContainsFunc(s.Bindings, func(b keymgr.Binding) bool { return b.Keys() == "ctrl+o" }) {
			return
		}
	}
	t.Fatalf("help sections %+v lack App ctrl+o", m.helpModel.KeySections)
}
