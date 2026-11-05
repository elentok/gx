// Package tickettest holds test helpers shared by tests of package tickets and
// the packages split out of it. It must not import tickets.
package tickettest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/testutil"
)

// WriteFile writes content to path, creating parent dirs. A file under an
// "issues" dir also gets its epic's ticket.md.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if issuesDir := filepath.Dir(path); filepath.Base(issuesDir) == "issues" {
		testutil.EnsureEpicTicketMD(t, filepath.Dir(issuesDir))
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// RequireWaitsForProjectMigration checks one writer through its public
// function. mk builds a store project under root and returns its dir with the
// write to run. With the first project's exclusive lock held, that write blocks
// and lands after release; the same write on a second project does not wait.
func RequireWaitsForProjectMigration(t *testing.T, mk func(t *testing.T, root string) (projectDir string, write func() error)) {
	t.Helper()
	dir, write := mk(t, t.TempDir())
	_, otherWrite := mk(t, t.TempDir())
	unlock, err := projectlock.Exclusive(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	if err := otherWrite(); err != nil {
		t.Fatalf("write on another project: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- write() }()
	select {
	case err := <-done:
		t.Fatalf("write returned while the project migrated: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write after release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("write did not proceed after release")
	}
}

// StoreProjectWithEpic makes <root>/proj with project.json and an epic whose
// ticket.md holds epicFM, and returns the project and epic dirs.
func StoreProjectWithEpic(t *testing.T, root, epicFM string) (projectDir, epicDir string) {
	t.Helper()
	projectDir = filepath.Join(root, "proj")
	epicDir = filepath.Join(projectDir, "epic")
	WriteFile(t, filepath.Join(projectDir, "project.json"), "{}")
	WriteFile(t, filepath.Join(epicDir, "ticket.md"), "---\n"+epicFM+"---\n\n# epic\n")
	return projectDir, epicDir
}
