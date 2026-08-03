package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui/nav"
)

func TestTicketsConfirmOpensQueueWithSharedSelection(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	issuesDir := filepath.Join(repoDir, ".scratch", "my-epic", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "01-first.md"), []byte("Status: open\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
	})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForAppText(t, tm, "my-epic")
	tm.Send(tea.KeyPressMsg{Code: 'j', Text: "j"})
	tm.Send(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	tm.Send(tea.KeyPressMsg{Code: 'i', Text: "i"})
	waitForAppText(t, tm, "Open the execution plan")

	tm.Send(tea.KeyPressMsg{Code: 'y', Text: "y"})
	waitForAppText(t, tm, "This is the execution plan")
	waitForAppText(t, tm, "First")
}

func waitForAppText(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(string(output), want)
	}, teatest.WithDuration(5*time.Second))
}
