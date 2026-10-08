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
	"github.com/elentok/gx/ui/nav"
)

// Without a server, "r" on the Tickets tab only asks the user to start one.
func TestTicketsReplaceQueueWithoutServerNotifies(t *testing.T) {
	// not parallel-safe: points XDG_DATA_HOME at a temp ticket store
	repoDir := testutil.TempRepo(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	project := filepath.Join(os.Getenv("XDG_DATA_HOME"), "gx", "tickets", "p")
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
	})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	waitForAppText(t, tm, "my-epic")
	tm.Send(tea.KeyPressMsg{Code: 'j', Text: "j"})
	tm.Send(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	tm.Send(tea.KeyPressMsg{Code: 'r', Text: "r"})

	waitForAppText(t, tm, "start the server to queue tickets")
}

func waitForAppText(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return strings.Contains(string(output), want)
	}, teatest.WithDuration(5*time.Second))
}
