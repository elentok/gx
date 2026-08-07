package tree

import "testing"

type fixtureNode struct {
	id       string
	children []fixtureNode
}

func fixtureChildren(nodes []fixtureNode) map[string][]fixtureNode {
	byID := map[string][]fixtureNode{}
	var walk func([]fixtureNode)
	walk = func(nodes []fixtureNode) {
		for _, n := range nodes {
			byID[n.id] = n.children
			walk(n.children)
		}
	}
	walk(nodes)
	return byID
}

func idOf(n fixtureNode) string { return n.id }

func TestBuildEntriesFromValues_FlatNoChildren(t *testing.T) {
	roots := []fixtureNode{{id: "b"}, {id: "a"}, {id: "c"}}
	entries := BuildEntriesFromValues(roots, idOf, func(fixtureNode) []fixtureNode { return nil }, nil)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// order-preserving, not sorted: b, a, c
	if entries[0].ID != "b" || entries[1].ID != "a" || entries[2].ID != "c" {
		t.Fatalf("unexpected order: %v %v %v", entries[0].ID, entries[1].ID, entries[2].ID)
	}
	for _, e := range entries {
		if e.HasChildren {
			t.Errorf("entry %q should have no children", e.ID)
		}
		if e.Depth != 0 {
			t.Errorf("entry %q depth = %d, want 0", e.ID, e.Depth)
		}
	}
}

func TestBuildEntriesFromValues_NestedTwoLevels(t *testing.T) {
	roots := []fixtureNode{
		{id: "root", children: []fixtureNode{
			{id: "child1"},
			{id: "child2", children: []fixtureNode{
				{id: "grandchild"},
			}},
		}},
	}
	byID := fixtureChildren(roots)
	childrenFn := func(n fixtureNode) []fixtureNode { return byID[n.id] }

	entries := BuildEntriesFromValues(roots, idOf, childrenFn, nil)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	want := []struct {
		id       string
		depth    int
		parentID string
	}{
		{"root", 0, ""},
		{"child1", 1, "root"},
		{"child2", 1, "root"},
		{"grandchild", 2, "child2"},
	}
	for i, w := range want {
		if entries[i].ID != w.id {
			t.Fatalf("entries[%d].ID = %q, want %q", i, entries[i].ID, w.id)
		}
		if entries[i].Depth != w.depth {
			t.Errorf("entries[%d].Depth = %d, want %d", i, entries[i].Depth, w.depth)
		}
		if entries[i].ParentID != w.parentID {
			t.Errorf("entries[%d].ParentID = %q, want %q", i, entries[i].ParentID, w.parentID)
		}
	}

	if !entries[0].HasChildren || !entries[0].Expanded {
		t.Error("root should have children and be expanded by default")
	}
	if entries[1].HasChildren {
		t.Error("child1 should have no children")
	}
	if !entries[2].HasChildren || !entries[2].Expanded {
		t.Error("child2 should have children and be expanded by default")
	}
	if entries[3].HasChildren {
		t.Error("grandchild should have no children")
	}
}

func TestBuildEntriesFromValues_CollapsedNodeHidesChildren(t *testing.T) {
	roots := []fixtureNode{
		{id: "root", children: []fixtureNode{
			{id: "child1"},
			{id: "child2"},
		}},
	}
	byID := fixtureChildren(roots)
	childrenFn := func(n fixtureNode) []fixtureNode { return byID[n.id] }

	collapsed := map[string]bool{"root": true}
	entries := BuildEntriesFromValues(roots, idOf, childrenFn, collapsed)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (collapsed root, children hidden), got %d", len(entries))
	}
	if entries[0].Expanded {
		t.Error("expected root to be collapsed")
	}
	if !entries[0].HasChildren {
		t.Error("expected root to still report HasChildren even while collapsed")
	}
}

func TestBuildEntriesFromValues_OrderPreservingNotSorted(t *testing.T) {
	roots := []fixtureNode{
		{id: "zebra", children: []fixtureNode{{id: "z-child"}}},
		{id: "apple"},
	}
	byID := fixtureChildren(roots)
	childrenFn := func(n fixtureNode) []fixtureNode { return byID[n.id] }

	entries := BuildEntriesFromValues(roots, idOf, childrenFn, nil)

	if entries[0].ID != "zebra" {
		t.Errorf("entries[0].ID = %q, want %q (caller order, not sorted)", entries[0].ID, "zebra")
	}
	if entries[1].ID != "z-child" {
		t.Errorf("entries[1].ID = %q, want %q", entries[1].ID, "z-child")
	}
	if entries[2].ID != "apple" {
		t.Errorf("entries[2].ID = %q, want %q", entries[2].ID, "apple")
	}
}

func TestBuildEntriesFromPaths_FlatFiles(t *testing.T) {
	paths := []string{"b.txt", "a.txt", "c.txt"}
	entries := BuildEntriesFromPaths(paths, func(s string) string { return s }, nil)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// sorted alphabetically: a.txt, b.txt, c.txt
	if entries[0].ID != "a.txt" {
		t.Errorf("entries[0].ID = %q, want 'a.txt'", entries[0].ID)
	}
}

func TestBuildEntriesFromPaths_NestedDirs(t *testing.T) {
	paths := []string{"src/main.go", "src/util.go", "README.md"}
	entries := BuildEntriesFromPaths(paths, func(s string) string { return s }, nil)

	// dirs come before files; expect src/ dir first, then README.md
	if !entries[0].HasChildren {
		t.Fatalf("expected first entry to be dir, got %+v", entries[0])
	}
	if entries[0].ID != "src" {
		t.Errorf("dir id = %q, want 'src'", entries[0].ID)
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

func TestBuildEntriesFromPaths_CollapsedDir(t *testing.T) {
	paths := []string{"src/main.go", "src/util.go"}
	collapsed := map[string]bool{"src": true}
	entries := BuildEntriesFromPaths(paths, func(s string) string { return s }, collapsed)

	// Only the dir entry, children hidden
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (collapsed dir), got %d", len(entries))
	}
	if !entries[0].HasChildren {
		t.Errorf("expected dir entry, got %+v", entries[0])
	}
	if entries[0].Expanded {
		t.Error("expected dir to be collapsed")
	}
}

func TestBuildEntriesFromPaths_CollapsedDirChain(t *testing.T) {
	// a/b/file.txt - a and b are single-child dirs, should be collapsed into "a/b"
	paths := []string{"a/b/file.txt"}
	entries := BuildEntriesFromPaths(paths, func(s string) string { return s }, nil)

	if len(entries) == 0 {
		t.Fatal("expected entries")
	}
	if !entries[0].HasChildren {
		t.Errorf("expected dir entry, got %+v", entries[0])
	}
	// DisplayName should be collapsed chain
	if entries[0].DisplayName != "a/b" {
		t.Errorf("expected collapsed dir name 'a/b', got %q", entries[0].DisplayName)
	}
}

func TestBuildEntriesFromPaths_LeavesCollected(t *testing.T) {
	paths := []string{"src/a.go", "src/b.go"}
	entries := BuildEntriesFromPaths(paths, func(s string) string { return s }, nil)

	var dirEntry *Entry[string]
	for i := range entries {
		if entries[i].HasChildren {
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
