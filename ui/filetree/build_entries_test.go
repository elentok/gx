package filetree

import (
	"testing"
)

func TestBuildEntriesFromValues_FlatFiles(t *testing.T) {
	paths := []string{"b.txt", "a.txt", "c.txt"}
	entries := BuildEntriesFromValues(paths, func(s string) string { return s }, nil)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// sorted alphabetically: a.txt, b.txt, c.txt
	if entries[0].Path != "a.txt" {
		t.Errorf("entries[0].Path = %q, want 'a.txt'", entries[0].Path)
	}
}

func TestBuildEntriesFromValues_NestedDirs(t *testing.T) {
	paths := []string{"src/main.go", "src/util.go", "README.md"}
	entries := BuildEntriesFromValues(paths, func(s string) string { return s }, nil)

	// dirs come before files; expect src/ dir first, then README.md
	if entries[0].Kind != EntryDir {
		t.Fatalf("expected first entry to be dir, got %v", entries[0].Kind)
	}
	if entries[0].Path != "src" {
		t.Errorf("dir path = %q, want 'src'", entries[0].Path)
	}
	// dir is expanded by default (not in collapsed map)
	if !entries[0].Expanded {
		t.Error("expected dir to be expanded by default")
	}
	// files inside dir
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 entries (dir + 2 files), got %d", len(entries))
	}
}

func TestBuildEntriesFromValues_CollapsedDir(t *testing.T) {
	paths := []string{"src/main.go", "src/util.go"}
	collapsed := map[string]bool{"src": true}
	entries := BuildEntriesFromValues(paths, func(s string) string { return s }, collapsed)

	// Only the dir entry, children hidden
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (collapsed dir), got %d", len(entries))
	}
	if entries[0].Kind != EntryDir {
		t.Errorf("expected EntryDir, got %v", entries[0].Kind)
	}
	if entries[0].Expanded {
		t.Error("expected dir to be collapsed")
	}
}

func TestBuildEntriesFromValues_CollapsedDirChain(t *testing.T) {
	// a/b/file.txt - a and b are single-child dirs, should be collapsed into "a/b"
	paths := []string{"a/b/file.txt"}
	entries := BuildEntriesFromValues(paths, func(s string) string { return s }, nil)

	// Should have dir "a/b" and file "a/b/file.txt"
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}
	if entries[0].Kind != EntryDir {
		t.Errorf("expected dir entry, got %v", entries[0].Kind)
	}
	// DisplayName should be collapsed chain
	if entries[0].DisplayName != "a/b" {
		t.Errorf("expected collapsed dir name 'a/b', got %q", entries[0].DisplayName)
	}
}

func TestBuildEntriesFromValues_LeavesCollected(t *testing.T) {
	paths := []string{"src/a.go", "src/b.go"}
	entries := BuildEntriesFromValues(paths, func(s string) string { return s }, nil)

	var dirEntry *Entry[string]
	for i := range entries {
		if entries[i].Kind == EntryDir {
			dirEntry = &entries[i]
			break
		}
	}
	if dirEntry == nil {
		t.Fatal("no dir entry found")
	}
	if len(dirEntry.Leaves) != 2 {
		t.Errorf("Leaves = %d, want 2", len(dirEntry.Leaves))
	}
}
