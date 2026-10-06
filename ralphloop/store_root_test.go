package ralphloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
)

// Seam B: a run with no explicit ticket dir reads its epic from the store.
// Not parallel-safe (sets env).
func TestRun_DefaultTicketDirIsTheStoreProject(t *testing.T) {
	repo := testutil.TempRepo(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	project := filepath.Join(os.Getenv("XDG_DATA_HOME"), "gx", "tickets", "p")
	issues := filepath.Join(project, "my-epic", "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"name": "p", "repo": repo})
	if err := os.WriteFile(filepath.Join(project, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issues, "01-first.md"), []byte("---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n"), 0644); err != nil {
		t.Fatal(err)
	}

	d, _, _ := fakeDeps()
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	d.Now = func() time.Time { return fixedNow }
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", RepoDir: repo}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if epic := loadEpicByName(t, project, "my-epic"); !epic.StartedAt.Equal(fixedNow) {
		t.Errorf("StartedAt = %v, want %v (epic read from the store)", epic.StartedAt, fixedNow)
	}
}
