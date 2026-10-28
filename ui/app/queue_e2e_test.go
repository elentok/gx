package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
)

// Without server deps the Tickets tab reads the store and "D" is disabled.
func TestTicketsFinishAndPauseWithoutServerIsDisabled(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	store := t.TempDir()
	project := filepath.Join(store, "p")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	pf, _ := json.Marshal(map[string]string{"name": "p", "repo": repoDir})
	if err := os.WriteFile(filepath.Join(project, "project.json"), pf, 0o644); err != nil {
		t.Fatal(err)
	}
	issuesDir := filepath.Join(project, "my-epic", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "my-epic"))
	if err := os.WriteFile(filepath.Join(issuesDir, "01-first.md"), []byte("---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}

	m := New(*repo, Settings{
		InitialRoute:       nav.ViewState{Tab: nav.TabTickets, WorktreeRoot: repoDir},
		ActiveWorktreePath: repoDir,
		Settings:           ui.Settings{TicketStore: store},
	})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForAppText(t, tm, "my-epic")
	tm.Send(tea.KeyPressMsg{Code: 'j', Text: "j"})
	tm.Send(tea.KeyPressMsg{Code: 'D', Text: "D"})

	waitForAppText(t, tm, "disabled: server is down")
}

func waitForAppText(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(string(output), want)
	}, teatest.WithDuration(5*time.Second))
}
