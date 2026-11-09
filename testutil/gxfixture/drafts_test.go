package main

import (
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
)

func TestDraftsScenario_SeedsThreeDraftsWithImages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := seedDrafts(dir, scenarios["drafts"].Drafts); err != nil {
		t.Fatal(err)
	}
	items, err := drafts.New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("drafts = %d, want 3", len(items))
	}
	images := 0
	for _, it := range items {
		images += it.ImageCount
	}
	if images != 2 {
		t.Errorf("images = %d, want 2", images)
	}
}
