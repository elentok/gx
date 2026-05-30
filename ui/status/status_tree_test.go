package status

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filetree"
)

func TestBuildStatusEntries_CollapsibleDirectory(t *testing.T) {
	files := []git.StageFileStatus{
		{Path: "dir/a.txt", IndexStatus: '?', WorktreeCode: '?'},
		{Path: "dir/b.txt", IndexStatus: '?', WorktreeCode: '?'},
		{Path: "root.txt", IndexStatus: ' ', WorktreeCode: 'M'},
	}

	entries := buildStatusEntries(files, map[string]bool{})
	if len(entries) < 4 {
		t.Fatalf("expected at least 4 entries, got %d", len(entries))
	}
	if entries[0].Kind != statusEntryDir || entries[0].Path != "dir" {
		t.Fatalf("expected first entry to be dir/, got %#v", entries[0])
	}
	if entries[1].Depth != 1 || entries[2].Depth != 1 {
		t.Fatalf("expected nested file depth=1, got %d and %d", entries[1].Depth, entries[2].Depth)
	}

	collapsed := buildStatusEntries(files, map[string]bool{"dir": true})
	if len(collapsed) != 2 {
		t.Fatalf("expected collapsed dir to hide children, got %d entries", len(collapsed))
	}
	if collapsed[0].Kind != statusEntryDir || collapsed[0].Expanded {
		t.Fatalf("expected collapsed dir row, got %#v", collapsed[0])
	}
}

func TestBuildStatusEntries_CompressesSingleChildDirectoryChains(t *testing.T) {
	files := []git.StageFileStatus{
		{Path: "keyboards/iris/keymaps/myfile.c", IndexStatus: ' ', WorktreeCode: 'M'},
	}

	entries := buildStatusEntries(files, map[string]bool{})
	if len(entries) != 2 {
		t.Fatalf("expected compressed dir plus file, got %d entries", len(entries))
	}
	if entries[0].Kind != statusEntryDir {
		t.Fatalf("expected first entry to be dir, got %#v", entries[0])
	}
	if entries[0].Path != "keyboards/iris/keymaps" {
		t.Fatalf("dir path = %q, want %q", entries[0].Path, "keyboards/iris/keymaps")
	}
	if entries[0].DisplayName != "keyboards/iris/keymaps" {
		t.Fatalf("dir display = %q, want %q", entries[0].DisplayName, "keyboards/iris/keymaps")
	}
	if entries[0].ParentPath != "" {
		t.Fatalf("dir parent = %q, want empty", entries[0].ParentPath)
	}
	if entries[1].Kind != statusEntryFile || entries[1].Path != "keyboards/iris/keymaps/myfile.c" {
		t.Fatalf("unexpected file entry %#v", entries[1])
	}
	if entries[1].ParentPath != "keyboards/iris/keymaps" {
		t.Fatalf("file parent = %q, want %q", entries[1].ParentPath, "keyboards/iris/keymaps")
	}
	if entries[1].Depth != 1 {
		t.Fatalf("file depth = %d, want 1", entries[1].Depth)
	}

	collapsed := buildStatusEntries(files, map[string]bool{"keyboards/iris/keymaps": true})
	if len(collapsed) != 1 {
		t.Fatalf("expected collapsed compressed dir to hide file, got %d entries", len(collapsed))
	}
	if collapsed[0].Kind != statusEntryDir || collapsed[0].Expanded {
		t.Fatalf("expected collapsed compressed dir row, got %#v", collapsed[0])
	}
}

func TestVisibleStatusLines_UsesStatusSpecificLabelAndMeta(t *testing.T) {
	m := Model{settings: ui.Settings{}, focus: focusFiletree, fileTreeModel: filetree.NewModel[git.StageFileStatus]()}
	file := git.StageFileStatus{Path: "new.go", RenameFrom: "old.go", IndexStatus: 'R', WorktreeCode: ' '}
	m.fileTreeModel.SetEntries([]filetree.Entry[git.StageFileStatus]{
		{Kind: filetree.EntryFile, DisplayName: "new.go", Value: file},
	})

	lines := m.visibleStatusLines(30, 3)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "old.go -> new.go") || !strings.Contains(got, "R") {
		t.Fatalf("line = %q, want rename path and status metadata", got)
	}
}
