package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
