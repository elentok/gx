package projects_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// appliedLines reads a project's list; ok is false when it has none.
func appliedLines(t *testing.T, projectDir string) (lines []string, ok bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectDir, migrations.AppliedFile))
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data)), true
}

func TestProjectAdd_NewProjectGetsTheFullAppliedList(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	res, err := h.Client.AddProject(context.Background(), wire.AddProjectRequest{Path: t.TempDir(), Name: "fresh", VCS: config.VCSNone})
	if err != nil || res.Refused {
		t.Fatalf("add = %+v, %v", res, err)
	}
	got, ok := appliedLines(t, filepath.Join(h.TicketStore, "fresh"))
	if !ok || !slices.Equal(got, migrations.Default.Names()) {
		t.Errorf("list = %v (exists %v), want %v", got, ok, migrations.Default.Names())
	}
}

func TestScratchProject_StartedServerGivesItTheFullList(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	got, ok := appliedLines(t, filepath.Join(h.TicketStore, projects.ScratchProject))
	if !ok || !slices.Equal(got, migrations.Default.Names()) {
		t.Errorf("scratch list = %v (exists %v), want %v", got, ok, migrations.Default.Names())
	}
}

func TestEnsureOutboxProject_NewGetsTheFullListAndExistingIsUntouched(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	if err := projects.EnsureOutboxProject(store, config.EvolveContributor); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(store, projects.OutboxProject)
	got, ok := appliedLines(t, dir)
	if !ok || !slices.Equal(got, migrations.Default.Names()) {
		t.Fatalf("new outbox list = %v (exists %v), want %v", got, ok, migrations.Default.Names())
	}
	if err := os.Remove(filepath.Join(dir, migrations.AppliedFile)); err != nil {
		t.Fatal(err)
	}
	if err := projects.EnsureOutboxProject(store, config.EvolveContributor); err != nil {
		t.Fatal(err)
	}
	if _, ok := appliedLines(t, dir); ok {
		t.Error("an existing outbox project gained a list")
	}
}
