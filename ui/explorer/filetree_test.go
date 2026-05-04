package explorer

import "testing"

func TestBuildFileTreeRowsCollapsibleDirectory(t *testing.T) {
	rows := BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "dir/a.txt", Value: "a"},
		{Path: "dir/b.txt", Value: "b"},
		{Path: "root.txt", Value: "root"},
	}, map[string]bool{})
	if len(rows) < 4 {
		t.Fatalf("expected at least 4 rows, got %d", len(rows))
	}
	if rows[0].Kind != FileTreeRowDir || rows[0].Path != "dir" {
		t.Fatalf("expected first row to be dir, got %#v", rows[0])
	}
	if rows[1].Depth != 1 || rows[2].Depth != 1 {
		t.Fatalf("expected nested file depth=1, got %d and %d", rows[1].Depth, rows[2].Depth)
	}

	collapsed := BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "dir/a.txt", Value: "a"},
		{Path: "dir/b.txt", Value: "b"},
		{Path: "root.txt", Value: "root"},
	}, map[string]bool{"dir": true})
	if len(collapsed) != 2 {
		t.Fatalf("expected collapsed dir to hide children, got %d rows", len(collapsed))
	}
	if collapsed[0].Kind != FileTreeRowDir || collapsed[0].Expanded {
		t.Fatalf("expected collapsed dir row, got %#v", collapsed[0])
	}
}

func TestBuildFileTreeRowsCompressesSingleChildChains(t *testing.T) {
	rows := BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "keyboards/iris/keymaps/myfile.c", Value: "leaf"},
	}, map[string]bool{})
	if len(rows) != 2 {
		t.Fatalf("expected compressed dir plus file, got %d", len(rows))
	}
	if rows[0].Kind != FileTreeRowDir {
		t.Fatalf("expected first row dir, got %#v", rows[0])
	}
	if rows[0].Path != "keyboards/iris/keymaps" {
		t.Fatalf("dir path = %q", rows[0].Path)
	}
	if rows[0].DisplayName != "keyboards/iris/keymaps" {
		t.Fatalf("dir display = %q", rows[0].DisplayName)
	}
	if rows[1].ParentPath != "keyboards/iris/keymaps" {
		t.Fatalf("file parent = %q", rows[1].ParentPath)
	}

	collapsed := BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "keyboards/iris/keymaps/myfile.c", Value: "leaf"},
	}, map[string]bool{"keyboards/iris/keymaps": true})
	if len(collapsed) != 1 {
		t.Fatalf("expected collapsed compressed dir to hide file, got %d", len(collapsed))
	}
}

func TestFileTreeNavigationHelpers(t *testing.T) {
	rows := BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "dir/a.txt", Value: "a"},
		{Path: "dir/b.txt", Value: "b"},
		{Path: "root.txt", Value: "root"},
	}, map[string]bool{})
	if idx, ok := FileTreeParentIndex(rows, 1); !ok || idx != 0 {
		t.Fatalf("parent index = %d, %v", idx, ok)
	}
	if idx, ok := FileTreeAdjacentFileIndex(rows, 1, 1); !ok || idx != 2 {
		t.Fatalf("adjacent next = %d, %v", idx, ok)
	}
	if idx, ok := FileTreeAdjacentFileIndex(rows, 2, -1); !ok || idx != 1 {
		t.Fatalf("adjacent prev = %d, %v", idx, ok)
	}

	collapsed := map[string]bool{}
	if !FileTreeCollapseSelectedDir(rows, collapsed, 0) {
		t.Fatal("expected collapse helper to change state")
	}
	if !collapsed["dir"] {
		t.Fatal("expected dir to be collapsed")
	}
	rows = BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "dir/a.txt", Value: "a"},
		{Path: "dir/b.txt", Value: "b"},
		{Path: "root.txt", Value: "root"},
	}, collapsed)
	if !FileTreeExpandSelectedDir(rows, collapsed, 0) {
		t.Fatal("expected expand helper to change state")
	}
	if collapsed["dir"] {
		t.Fatal("expected dir collapse state cleared")
	}
	rows = BuildFileTreeRows([]FileTreeLeaf[string]{
		{Path: "dir/a.txt", Value: "a"},
		{Path: "dir/b.txt", Value: "b"},
		{Path: "root.txt", Value: "root"},
	}, collapsed)
	if !FileTreeToggleDirOnEnter(rows, collapsed, 0) {
		t.Fatal("expected toggle helper to change state")
	}
	if !collapsed["dir"] {
		t.Fatal("expected toggle helper to collapse dir")
	}
}
