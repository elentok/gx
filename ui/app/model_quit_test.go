package app

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func newQuitTestApp(t *testing.T) Model {
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
	m.ensureLivePages()
	return m
}

// Quitting is never guarded: loops live in the server, not this process.
func TestQuitIsNeverGuarded(t *testing.T) {
	t.Parallel()
	for name, msg := range map[string]tea.Msg{
		"ctrl+c":       nav.ForceQuit()(),
		"back on root": nav.Back()(),
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := newQuitTestApp(t).Update(msg)
			if cmd == nil {
				t.Fatal("expected quit cmd")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("expected quit msg")
			}
		})
	}
}
