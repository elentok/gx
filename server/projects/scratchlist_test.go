package projects_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server/projects"
)

func TestEnsureScratchProject_NewGetsTheFullListAndExistingIsUntouched(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "store")
	listPath := filepath.Join(store, projects.ScratchProject, migrations.AppliedFile)
	read := func() ([]string, bool) {
		data, err := os.ReadFile(listPath)
		if os.IsNotExist(err) {
			return nil, false
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(data)), true
	}

	if err := projects.EnsureScratchProject(store); err != nil {
		t.Fatal(err)
	}
	if got, ok := read(); !ok || !slices.Equal(got, migrations.Default.Names()) {
		t.Fatalf("new list = %v (exists %v), want %v", got, ok, migrations.Default.Names())
	}

	if err := os.WriteFile(listPath, []byte("only-this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := projects.EnsureScratchProject(store); err != nil {
		t.Fatal(err)
	}
	if got, _ := read(); !slices.Equal(got, []string{"only-this"}) {
		t.Errorf("existing list = %v, want it untouched", got)
	}

	if err := os.Remove(listPath); err != nil {
		t.Fatal(err)
	}
	if err := projects.EnsureScratchProject(store); err != nil {
		t.Fatal(err)
	}
	if _, ok := read(); ok {
		t.Error("a project with no list gained one")
	}
}
