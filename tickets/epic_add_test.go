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
	for _, key := range []string{"pr_number: 7", "pr_url: https://example.com/pr/7", "archived_at: 2026-01-02T03:04:05Z", "archived_workspace: w", "branch_name: foo", "range_base: abc", "range_tip: abc"} {
		_, err := ParseEpicFields("kind: loop\n" + key + "\n")
		if err == nil || !strings.Contains(err.Error(), "server-stamped") {
			t.Errorf("%s: err = %v, want server-stamped refusal", key, err)
		}
	}
}

func TestCreateEpic_ThirteenthAreaRefusesAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	conv := t.TempDir()
	// Maps, loops and archived areas do not count toward the cap.
	for _, name := range []string{"m", "l"} {
		kind := KindMap
		if name == "l" {
			kind = KindLoop
		}
		if _, err := CreateEpic(conv, root, name, EpicFields{Kind: kind, Status: "open"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, ".archive", "old", "ticket.md"), "---\nkind: area\nstatus: open\n---\n# old\n")
	for i := 0; i < MaxAreas; i++ {
		name := "area-" + string(rune('a'+i))
		if _, err := CreateEpic(conv, root, name, EpicFields{Kind: KindArea, Status: "open"}, ""); err != nil {
			t.Fatalf("area %d: %v", i+1, err)
		}
	}

	_, err := CreateEpic(conv, root, "one-too-many", EpicFields{Kind: KindArea, Status: "open"}, "")
	if err == nil || !strings.Contains(err.Error(), "areas") {
		t.Fatalf("err = %v, want the area-limit refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "one-too-many")); statErr == nil {
		t.Error("refusal wrote an epic directory")
	}
	if _, err := CreateEpic(conv, root, "another-loop", EpicFields{Kind: KindLoop}, ""); err != nil {
		t.Errorf("a loop at the area cap: %v", err)
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
