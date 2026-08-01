package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

func writeEpicDir(t *testing.T, worktreeRoot, epic string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(worktreeRoot, ".scratch", epic, "issues"), 0755); err != nil {
		t.Fatal(err)
	}
}

// TestTicketsTabStaysAggregatedAcrossWorktreeSwitchMidLoop covers ticket 05:
// a ralph-loop launched against one worktree's epic must stay visible in the
// tickets tab even after the worktree cursor moves elsewhere and the tab is
// rebuilt from scratch (ADR-0007's worktree-cursor propagation would
// otherwise silently rescope to the newly-selected worktree only).
//
// isTicketsLoopRunning is stubbed rather than driving a real ralph-loop
// launch (see ui/tickets/implement_e2e_test.go), since a real launch against
// a zero-ticket epic completes almost immediately and would make "loop still
// running" a race rather than a deterministic precondition.
func TestTicketsTabStaysAggregatedAcrossWorktreeSwitchMidLoop(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtA := filepath.Join(repoDir, "feature-a")
	wtB := filepath.Join(repoDir, "feature-b")
	writeEpicDir(t, wtA, "epic-a")
	writeEpicDir(t, wtB, "epic-b")

	repo, err := git.FindRepo(wtA)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: wtA},
		ActiveWorktreePath: wtA,
	})

	isTicketsLoopRunning = func() bool { return true }
	t.Cleanup(func() { isTicketsLoopRunning = ticketsui.IsLoopRunning })

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	// Simulate moving the worktree cursor to feature-b in the worktrees tab and
	// then revisiting the tickets tab (mirrors model_test.go's existing pattern
	// of firing nav.Switch directly rather than driving the worktrees tab's own
	// cursor keys).
	tm.Send(nav.Switch(nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: wtB})())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "epic-a") && strings.Contains(s, "epic-b")
	}, teatest.WithDuration(3*time.Second))

	// Once the loop finishes, moving worktrees again should revert to
	// single-worktree scoping.
	isTicketsLoopRunning = func() bool { return false }
	tm.Send(nav.Switch(nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: wtA})())

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "epic-a") && !strings.Contains(s, "epic-b")
	}, teatest.WithDuration(3*time.Second))
}
