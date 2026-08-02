package app

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// TestLoopStatusOverlayVisibleOnNonTicketsTab covers ticket 06b: a
// ralph-loop's cross-tab status overlay must render on every tab, not just
// the tickets tab that launched it, and disappear once the loop stops.
func TestLoopStatusOverlayVisibleOnNonTicketsTab(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabWorktrees},
		ActiveWorktreePath: repoDir,
	})

	ticketsui.SetLoopStatusForTest("my-epic", 3, 7)
	t.Cleanup(ticketsui.ClearLoopStatusForTest)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		s := string(bts)
		return strings.Contains(s, "Implementing my-epic") && strings.Contains(s, "3/7")
	}, teatest.WithDuration(3*time.Second))

	ticketsui.ClearLoopStatusForTest()

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return !strings.Contains(string(bts), "Implementing my-epic")
	}, teatest.WithDuration(3*time.Second))
}
