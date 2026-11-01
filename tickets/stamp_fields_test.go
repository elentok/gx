package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/projectlock"
)

func TestStampEpicFields_KeepsOtherKeysAndNeverOverwrites(t *testing.T) {
	epic := t.TempDir()
	path := filepath.Join(epic, "ticket.md")
	original := "---\nstatus: open\nbase: main\ncompleted_at: 2026-01-02T03:04:05Z\n---\n\n# epic\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	err := StampEpicFields(epic, map[string]any{
		"completed_at": time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		"retro_at":     time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"status: open", "base: main", "completed_at: 2026-01-02T03:04:05Z", "retro_at: 2026-05-06T07:08:09Z", "# epic"} {
		if !strings.Contains(got, want) {
			t.Errorf("ticket.md missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2030") {
		t.Errorf("existing completed_at was overwritten:\n%s", got)
	}
}

func TestStampEpicFields_WaitsForProjectMigration(t *testing.T) {
	requireWaitsForProjectMigration(t, func(t *testing.T, root string) (string, func() error) {
		project, epic := storeProjectWithEpic(t, root, "status: open\n")
		return project, func() error {
			return StampEpicFields(epic, map[string]any{"completed_at": time.Now().UTC()})
		}
	})
}

func TestStampEpicFieldsInner_RunsWhileProjectLockHeld(t *testing.T) {
	project, epic := storeProjectWithEpic(t, t.TempDir(), "status: open\n")
	unlock, err := projectlock.Exclusive(project)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	done := make(chan error, 1)
	go func() { done <- StampEpicFieldsInner(epic, map[string]any{"merged_at": time.Now().UTC()}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("inner stamp waited for the migration lock")
	}
}

func TestClearEpicFields_RemovesOnlyTheNamedKeys(t *testing.T) {
	epic := t.TempDir()
	path := filepath.Join(epic, "ticket.md")
	original := "---\nstatus: open\nland_block_kind: dirty-trunk\nland_block_reason: trunk dirty\nbase: main\n---\n\n# epic\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearEpicFields(epic, "land_block_kind", "land_block_reason", "absent"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "---\nstatus: open\nbase: main\n---\n\n# epic\n"; string(raw) != want {
		t.Errorf("ticket.md =\n%q\nwant\n%q", raw, want)
	}
}
