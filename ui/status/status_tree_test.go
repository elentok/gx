package stage

import (
	"testing"

	"github.com/elentok/gx/git"
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
