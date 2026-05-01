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

func TestPushStatusAndBackRestoresPreviousPage(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.Route{Kind: nav.RouteWorktrees},
		ActiveWorktreePath: repoDir,
	})

	updated, cmd := m.Update(nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: repoDir})())
	if cmd == nil {
		t.Fatalf("expected init/resize cmd when pushing route")
	}
	m = updated.(Model)
	if m.current.route.Kind != nav.RouteStatus {
		t.Fatalf("expected current route status, got %q", m.current.route.Kind)
	}
	if len(m.history) != 1 {
		t.Fatalf("expected history depth 1, got %d", len(m.history))
	}

	updated, cmd = m.Update(nav.Back()())
	m = updated.(Model)
	if m.current.route.Kind != nav.RouteWorktrees {
		t.Fatalf("expected current route worktrees after back, got %q", m.current.route.Kind)
	}
	if len(m.history) != 0 {
		t.Fatalf("expected empty history after back, got %d", len(m.history))
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
