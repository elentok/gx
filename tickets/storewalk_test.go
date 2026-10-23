package tickets

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeWalkEpic(t *testing.T, parent, name string) {
	t.Helper()
	dir := filepath.Join(parent, name, "issues")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "01-a.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStoreEpics_ReturnsLiveAndArchivedEpicsPerProject(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	alpha := filepath.Join(store, "alpha")
	beta := filepath.Join(store, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeWalkEpic(t, alpha, "live")
	writeWalkEpic(t, alpha, "follow-ups-archived")
	writeWalkEpic(t, alpha, "one-offs-archived")
	writeWalkEpic(t, filepath.Join(alpha, ".archive"), "old")
	writeWalkEpic(t, beta, "other")
	// No project.json: skipped.
	writeWalkEpic(t, filepath.Join(store, "stray"), "ignored")

	got, err := LoadStoreEpics(store)
	if err != nil {
		t.Fatalf("LoadStoreEpics: %v", err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Project+"/"+e.Epic.Name)
	}
	sort.Strings(names)
	want := []string{
		"alpha/follow-ups-archived",
		"alpha/live",
		"alpha/old",
		"alpha/one-offs-archived",
		"beta/other",
	}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
	for _, e := range got {
		if e.Archived != (e.Epic.Name == "old") {
			t.Errorf("%s/%s: Archived = %v", e.Project, e.Epic.Name, e.Archived)
		}
	}
}
