package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_TicketsEpics_ListsBareSlugsSortedExcludingArchive(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repo)
	for _, name := range []string{"zebra-epic", "alpha-epic"} {
		testutil.EnsureEpicTicketMD(t, filepath.Join(project, name))
	}
	testutil.Mkdir(t, filepath.Join(project, ".archive"))
	// A stray file directly under the project should not be treated as an epic.
	testutil.WriteFile(t, project, "notes.txt", "not an epic")

	out, err := runIn(t, repo, "tickets", "epics")
	if err != nil {
		t.Fatalf("execute tickets epics: %v", err)
	}
	if want := "alpha-epic\nzebra-epic\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestExecute_TicketsEpics_MapsFlagFiltersToWayfinderMapEpics(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repo)
	for _, name := range []string{"zebra-epic", "alpha-epic"} {
		if err := os.MkdirAll(filepath.Join(project, name), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	testutil.WriteFile(t, project, "alpha-epic/map.md", "# alpha map")

	out, err := runIn(t, repo, "tickets", "epics", "--maps")
	if err != nil {
		t.Fatalf("execute tickets epics --maps: %v", err)
	}
	if want := "alpha-epic\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestExecute_TicketsEpics_EmptyProjectExitsZeroWithNoOutput(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	addProject(t, store, "mine", repo)

	out, err := runIn(t, repo, "tickets", "epics")
	if err != nil {
		t.Fatalf("execute tickets epics: %v", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
}

func TestExecute_TicketsEpics_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := runIn(t, dir, "tickets", "epics")
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a git repo") {
		t.Errorf("error = %q, want it to mention not being inside a git repo", err.Error())
	}
}
