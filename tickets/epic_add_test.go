package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateEpic_WaitsForProjectMigration(t *testing.T) {
	requireWaitsForProjectMigration(t, func(t *testing.T, root string) (string, func() error) {
		project := filepath.Join(root, "proj")
		writeFile(t, filepath.Join(project, "project.json"), "{}")
		return project, func() error {
			_, err := CreateEpic(t.TempDir(), project, "new-epic", EpicFields{Kind: KindLoop}, "")
			return err
		}
	})
}

func TestParseEpicFields_RefusesPRFields(t *testing.T) {
	for _, key := range []string{"pr_number: 7", "pr_url: https://example.com/pr/7", "archived_at: 2026-01-02T03:04:05Z", "archived_workspace: w", "branch_name: foo"} {
		_, err := ParseEpicFields("kind: loop\n" + key + "\n")
		if err == nil || !strings.Contains(err.Error(), "server-stamped") {
			t.Errorf("%s: err = %v, want server-stamped refusal", key, err)
		}
	}
}

func TestCreateEpic_RejectsCallerCreatedAtAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	fields := EpicFields{Kind: KindLoop, CreatedAt: time.Now()}

	_, err := CreateEpic(t.TempDir(), root, "x", fields, "")
	if err == nil || !strings.Contains(err.Error(), "server-stamped") {
		t.Fatalf("err = %v, want server-stamped refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "x")); statErr == nil {
		t.Error("refusal wrote an epic directory")
	}
}
