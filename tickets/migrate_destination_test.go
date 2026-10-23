package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateDestinationUnconfirmed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(name, fm string) string {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "ticket.md")
		if err := os.WriteFile(path, []byte(fm), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	old := write("old", "---\nkind: map\nprovisional_destination: true\n---\n\n# old\n")
	both := write("both", "---\nkind: map\nprovisional_destination: true\ndestination_unconfirmed: false\n---\n\n# both\n")
	clean := "---\nkind: map\ndestination_unconfirmed: true\n---\n\n# clean\n"
	cleanPath := write("clean", clean)

	changes, err := MigrateDestinationUnconfirmed(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %v, want old and both", changes)
	}

	raw, _ := os.ReadFile(old)
	if got := string(raw); !strings.Contains(got, "destination_unconfirmed: true") || strings.Contains(got, "provisional_destination") || !strings.Contains(got, "# old") {
		t.Errorf("old not renamed with value kept:\n%s", got)
	}
	raw, _ = os.ReadFile(both)
	if got := string(raw); !strings.Contains(got, "destination_unconfirmed: false") || strings.Contains(got, "provisional_destination") {
		t.Errorf("both: the new key should win and the old one go:\n%s", got)
	}
	if raw, _ = os.ReadFile(cleanPath); string(raw) != clean {
		t.Errorf("clean epic rewritten:\n%s", raw)
	}

	again, err := MigrateDestinationUnconfirmed(root)
	if err != nil || len(again) != 0 {
		t.Errorf("rerun = %v, %v; want no changes", again, err)
	}
}
