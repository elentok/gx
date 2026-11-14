package main

import (
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/testutil/gxfixture/catalog"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestDraftsScenario_SeedDraftsSkipsLateDraftsBeforeStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := seedDrafts(dir, catalog.Scenarios["bulk-actions"].Drafts); err != nil {
		t.Fatal(err)
	}
	items, err := drafts.New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != spec.DraftSlugBulk {
		t.Fatalf("pre-start drafts = %+v, want only %q", items, spec.DraftSlugBulk)
	}
}

func TestDraftsScenario_SeedsThreeDraftsWithImages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := seedDrafts(dir, catalog.Scenarios["drafts"].Drafts); err != nil {
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
