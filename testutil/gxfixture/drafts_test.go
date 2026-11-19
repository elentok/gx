package main

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
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

func TestDraftHistoryScenario_SeedsThreeListsNewestFirst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := seedDraftHistory(dir, catalog.Scenarios["draft-history"].History); err != nil {
		t.Fatal(err)
	}
	store := drafthistory.New(dir)
	byText := func(l drafthistory.List) map[string]drafthistory.Item {
		items, err := store.List(l)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) < 6 {
			t.Fatalf("%s holds %d entries, want at least 6", l, len(items))
		}
		withImages, without := 0, 0
		out := map[string]drafthistory.Item{}
		for _, it := range items {
			out[it.Text] = it
			if it.ImageCount > 0 || it.ImagesDropped {
				withImages++
			} else {
				without++
			}
		}
		if withImages == 0 || without == 0 {
			t.Errorf("%s: %d entries with images, %d without, want both", l, withImages, without)
		}
		return out
	}

	design := byText(drafthistory.DesignDrafts)
	if got := design[spec.HistoryDesignExists].Slug; got != spec.HistoryDesignSlug {
		t.Errorf("existing-Design entry slug = %q, want %q", got, spec.HistoryDesignSlug)
	}
	if got := design[spec.HistoryDesignOther].Project; got != spec.HistoryOtherProject {
		t.Errorf("other-project entry project = %q, want %q", got, spec.HistoryOtherProject)
	}
	if dropped := design[spec.HistoryDesignDropped]; !dropped.ImagesDropped || dropped.ImageCount != 0 {
		t.Errorf("dropped entry = %+v, want the images-not-kept mark and no images", dropped)
	}
	gone := design[spec.HistoryDesignGone]
	paths, err := store.ImagePaths(drafthistory.DesignDrafts, gone.ID)
	if err != nil || gone.ImageCount != 1 || len(paths) != 1 {
		t.Fatalf("deleted-image entry = %+v, paths %v, err %v, want a record listing one image", gone, paths, err)
	}
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Errorf("deleted image: stat err = %v, want not exist", err)
	}

	chat := byText(drafthistory.ChatDrafts)
	harnesses, models, efforts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, it := range chat {
		harnesses[it.Harness], models[it.Model], efforts[it.Effort] = true, true, true
	}
	if len(harnesses) < 2 || len(models) < 2 || len(efforts) < 2 {
		t.Errorf("chat drafts: harnesses %v, models %v, efforts %v, want each to differ", harnesses, models, efforts)
	}

	sent := byText(drafthistory.Sent)
	composers := map[drafthistory.Composer]bool{}
	for _, it := range sent {
		composers[it.Composer] = true
	}
	if !composers[drafthistory.ComposerChat] || !composers[drafthistory.ComposerDesign] {
		t.Errorf("sent composers = %v, want chat and design", composers)
	}
	if it := sent[spec.HistorySentImages]; it.ImageCount == 0 {
		t.Errorf("images-only entry = %+v, want images and no text", it)
	}
	if it := sent[spec.HistorySentCommand]; it.Harness != "claude" || it.Composer != drafthistory.ComposerChat {
		t.Errorf("command entry = %+v, want a Claude chat entry", it)
	}
}

func TestDraftHistoryScenario_StartsWithPagesAndIdleSessions(t *testing.T) {
	t.Parallel()
	r := start(t, "draft-history")
	var snap wire.Snapshot
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "snapshot", "--json")), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.DesignDrafts) != 1 || len(snap.ChatDrafts) != 1 {
		t.Errorf("drafts: design %+v, chat %+v, want one each", snap.DesignDrafts, snap.ChatDrafts)
	}
	var sessions []wire.SessionInfo
	// The Queue agent is started by the queue loop, so it can show up a moment after start.
	testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(sessions, func(s wire.SessionInfo) bool { return s.Queue })
	})
	var plainClaude, plainCodex, designs, fixes, queues int
	for _, s := range sessions {
		switch {
		case s.Queue:
			queues++
			continue
		case s.FixOf != "":
			fixes++
		case s.Design:
			designs++
		case s.Harness == "codex":
			plainCodex++
		default:
			plainClaude++
		}
		if s.Status.State != wire.SessionIdle {
			t.Errorf("session %s (%s) is %v, want idle", s.ID, s.Name, s.Status.State)
		}
	}
	if plainClaude != 1 || plainCodex != 1 || designs != 1 || fixes != 1 || queues != 1 {
		t.Errorf("sessions: claude %d, codex %d, design %d, fix %d, queue %d, want one each: %+v",
			plainClaude, plainCodex, designs, fixes, queues, sessions)
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
