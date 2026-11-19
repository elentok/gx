package drafthistory_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/agentfake"
)

// pngOf makes a distinct valid PNG per width.
func pngOf(w int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, 1)))
	return b.Bytes()
}

func writePNGs(t *testing.T, widths ...int) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for _, w := range widths {
		p := filepath.Join(dir, "img-"+strings.Repeat("a", w)+".png")
		if err := os.WriteFile(p, pngOf(w), 0o600); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// seedEntry saves e to list l and returns the new entry's id.
func seedEntry(t *testing.T, f rigs.DesignsFixture, l drafthistory.List, e drafthistory.Entry, widths ...int) string {
	t.Helper()
	s := drafthistory.New(f.H.StateDir)
	if _, err := s.Save(l, e, writePNGs(t, widths...)); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(l)
	if err != nil {
		t.Fatal(err)
	}
	return items[0].ID
}

func listOf(t *testing.T, f rigs.DesignsFixture, l drafthistory.List) []drafthistory.Item {
	t.Helper()
	items, err := drafthistory.New(f.H.StateDir).List(l)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func postPick(t *testing.T, f rigs.DesignsFixture, path string, req ops.DraftHistoryPickRequest, into any) {
	t.Helper()
	b, _ := json.Marshal(req)
	code, out := servertest.WebPostJSON(t, f.Base+path, string(b), f.Ck, "", servertest.JSONPost(f.Base))
	if code != http.StatusOK {
		t.Fatalf("POST %s = %d %s", path, code, out)
	}
	if err := json.Unmarshal([]byte(out), into); err != nil {
		t.Fatal(err)
	}
}

func pickDesign(t *testing.T, f rigs.DesignsFixture, entry string, d *ops.DesignDraft) ops.DesignDraftPickResult {
	t.Helper()
	var res ops.DesignDraftPickResult
	postPick(t, f, "/v1/designs/drafts/pick-history", ops.DraftHistoryPickRequest{EntryID: entry, DraftID: d.ID, Revision: d.Revision}, &res)
	return res
}

// designPage makes a Design draft with the given widths of images.
func designPage(t *testing.T, f rigs.DesignsFixture, project, slug, idea string, widths ...int) *ops.DesignDraft {
	t.Helper()
	st := drafts.New(f.H.StateDir)
	d, err := st.Create(project, slug, idea)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range widths {
		if res, err := st.AddImage(d.ID, pngOf(w)); err != nil || res.Refused {
			t.Fatalf("add image: %+v %v", res, err)
		}
	}
	d, err = st.Get(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func imageFiles(t *testing.T, folder string) []string {
	t.Helper()
	es, err := os.ReadDir(folder)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestPickDraft_DesignReplacesTextImagesAndSlugAndKeepsProject(t *testing.T) {
	f := rigs.StartDesigns(t)
	st := drafts.New(f.H.StateDir)
	d := designPage(t, f, "proj", "old-slug", "old idea", 3)
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked idea", Project: "elsewhere", Slug: "new-slug"}, 5, 6)

	res := pickDesign(t, f, entry, d)

	if res.Refused || res.Draft == nil || len(res.Notices) != 0 {
		t.Fatalf("res = %+v", res)
	}
	got := res.Draft
	if got.Idea != "picked idea" || got.Slug != "new-slug" || got.Project != "proj" || got.Revision != d.Revision+1 || len(got.Images) != 2 {
		t.Errorf("draft = %+v", got)
	}
	disk, _ := st.Get(d.ID)
	if disk.Revision != got.Revision || !slices.Equal(disk.Images, got.Images) {
		t.Errorf("disk = %+v, result = %+v", disk, got)
	}
	if files := imageFiles(t, st.ImagesDir(d.ID)); len(files) != 2 {
		t.Errorf("image folder = %v, want only the 2 picked images", files)
	}
	if _, ok := imagestore.Find(st.ImagesDir(d.ID), d.Images[0]); ok {
		t.Error("the page's old image is still in the folder")
	}
	items := listOf(t, f, drafthistory.DesignDrafts)
	if len(items) != 2 || items[0].Text != "old idea" || items[0].Slug != "old-slug" || items[0].ImageCount != 1 {
		t.Errorf("list = %+v", items)
	}
}

func TestPickDraft_EmptyPageAddsNoEntry(t *testing.T) {
	f := rigs.StartDesigns(t)
	d := designPage(t, f, "proj", "", "")
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked"})

	res := pickDesign(t, f, entry, d)

	if res.Draft == nil || res.Draft.Idea != "picked" {
		t.Fatalf("res = %+v", res)
	}
	if items := listOf(t, f, drafthistory.DesignDrafts); len(items) != 1 {
		t.Errorf("list = %+v, want only the seeded entry", items)
	}
}

func TestPickDraft_EntryEqualToThePageChangesNothing(t *testing.T) {
	f := rigs.StartDesigns(t)
	d := designPage(t, f, "proj", "same", "same text", 4)
	// The entry's project differs: a pick never changes the project, so it does not count.
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "same text", Project: "elsewhere", Slug: "same"}, 4)

	res := pickDesign(t, f, entry, d)

	if res.Refused || res.Draft == nil || res.Draft.Revision != d.Revision || !slices.Equal(res.Draft.Images, d.Images) {
		t.Fatalf("res = %+v, page = %+v", res, d)
	}
	if items := listOf(t, f, drafthistory.DesignDrafts); len(items) != 1 {
		t.Errorf("list = %+v", items)
	}
}

func TestPickDraft_StaleRevisionAndMissingDraftAreRefusedAndSaveNothing(t *testing.T) {
	f := rigs.StartDesigns(t)
	st := drafts.New(f.H.StateDir)
	d := designPage(t, f, "proj", "", "page text", 3)
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked"}, 5)

	stale := *d
	stale.Revision--
	if res := pickDesign(t, f, entry, &stale); !res.Refused || res.Reason != ops.ReasonDraftRevisionConflict {
		t.Errorf("stale = %+v", res)
	}
	disk, _ := st.Get(d.ID)
	if disk.Idea != "page text" || disk.Revision != d.Revision || !slices.Equal(disk.Images, d.Images) {
		t.Errorf("draft changed: %+v", disk)
	}
	if files := imageFiles(t, st.ImagesDir(d.ID)); len(files) != 1 {
		t.Errorf("images = %v", files)
	}
	if items := listOf(t, f, drafthistory.DesignDrafts); len(items) != 1 {
		t.Errorf("list = %+v", items)
	}

	gone := *d
	gone.ID = "0123456789abcdef"
	if res := pickDesign(t, f, entry, &gone); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("gone = %+v", res)
	}
	if items := listOf(t, f, drafthistory.DesignDrafts); len(items) != 1 {
		t.Errorf("list = %+v", items)
	}
}

func TestPickDraft_TakenSlugIsClearedWithANotice(t *testing.T) {
	f := rigs.StartDesigns(t)
	d := designPage(t, f, "proj", "", "page text")
	// The fixture project has an epic named "map".
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked", Project: "proj", Slug: "map"})

	res := pickDesign(t, f, entry, d)

	if res.Draft == nil || res.Draft.Slug != "" || res.Draft.Idea != "picked" {
		t.Fatalf("draft = %+v", res.Draft)
	}
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "map") {
		t.Errorf("notices = %v", res.Notices)
	}
}

func TestPickDraft_DeletedImageFileIsSkippedAndCounted(t *testing.T) {
	f := rigs.StartDesigns(t)
	d := designPage(t, f, "proj", "", "page text")
	entry := seedEntry(t, f, drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked"}, 3, 4, 5)
	s := drafthistory.New(f.H.StateDir)
	paths, err := s.ImagePaths(drafthistory.DesignDrafts, entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths[1]); err != nil {
		t.Fatal(err)
	}

	res := pickDesign(t, f, entry, d)

	if res.Draft == nil || len(res.Draft.Images) != 2 {
		t.Fatalf("draft = %+v", res.Draft)
	}
	if len(res.Notices) != 1 || res.Notices[0] != "1 images missing" {
		t.Errorf("notices = %v", res.Notices)
	}
}

func TestPickDraft_ImagesNotKeptRestoresTheTextOnly(t *testing.T) {
	f := rigs.StartDesigns(t)
	d := designPage(t, f, "proj", "", "page text")
	s := drafthistory.NewWith(f.H.StateDir, 1, nil)
	if _, err := s.Save(drafthistory.DesignDrafts, drafthistory.Entry{Text: "picked", Slug: "kept-slug"}, writePNGs(t, 5)); err != nil {
		t.Fatal(err)
	}
	entry := listOf(t, f, drafthistory.DesignDrafts)[0]
	if !entry.ImagesDropped {
		t.Fatal("setup: entry should be marked images not kept")
	}

	res := pickDesign(t, f, entry.ID, d)

	if res.Draft == nil || res.Draft.Idea != "picked" || res.Draft.Slug != "kept-slug" || len(res.Draft.Images) != 0 || len(res.Notices) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestPickDraft_EntryRestoresAfterItsSourceDraftIsGone(t *testing.T) {
	f := rigs.StartDesigns(t)
	st := drafts.New(f.H.StateDir)
	src := designPage(t, f, "proj", "src", "source text", 7)
	if err := st.DeleteSaving(src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.ImagesDir(src.ID)); !os.IsNotExist(err) {
		t.Fatalf("source folder still there: %v", err)
	}
	entry := listOf(t, f, drafthistory.DesignDrafts)[0].ID
	d := designPage(t, f, "proj", "", "other")

	res := pickDesign(t, f, entry, d)

	if res.Draft == nil || res.Draft.Idea != "source text" || len(res.Draft.Images) != 1 || len(res.Notices) != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestPickDraft_ChatReplacesTextImagesAndNameAndKeepsSettings(t *testing.T) {
	f := rigs.StartDesigns(t)
	st := chatdrafts.New(f.H.StateDir)
	var created ops.ChatDraftResult
	body, _ := json.Marshal(ops.ChatDraftCreateRequest{Project: "proj", Brief: "page brief", Name: "page-name", Folder: ops.ChatDraftFolderTrunk, Harness: string(agentfake.HarnessKind), Model: "page-model", Effort: "high"})
	if code, out := servertest.WebPostJSON(t, f.Base+"/v1/agents/drafts/create", string(body), f.Ck, "", servertest.JSONPost(f.Base)); code != http.StatusOK || json.Unmarshal([]byte(out), &created) != nil || created.Draft == nil {
		t.Fatalf("create = %d %s", code, out)
	}
	page := created.Draft
	if res, err := st.AddImage(page.ID, pngOf(3)); err != nil || res.Refused {
		t.Fatalf("add image: %+v %v", res, err)
	}
	page, _ = st.Get(page.ID)
	entry := seedEntry(t, f, drafthistory.ChatDrafts, drafthistory.Entry{
		Text: "picked brief", Name: "picked-name", Project: "other", Folder: "worktree", Harness: "h2", Model: "m2", Effort: "low",
	}, 5, 6)

	var res ops.ChatDraftPickResult
	postPick(t, f, "/v1/agents/drafts/pick-history", ops.DraftHistoryPickRequest{EntryID: entry, DraftID: page.ID, Revision: page.Revision}, &res)

	got := res.Draft
	if res.Refused || got == nil {
		t.Fatalf("res = %+v", res)
	}
	if got.Brief != "picked brief" || got.Name != "picked-name" || len(got.Images) != 2 || got.Revision != page.Revision+1 {
		t.Errorf("draft = %+v", got)
	}
	if got.Project != "proj" || got.Folder != page.Folder || got.Harness != string(agentfake.HarnessKind) || got.Model != "page-model" || got.Effort != "high" {
		t.Errorf("settings changed: %+v", got)
	}
	if files := imageFiles(t, st.ImagesDir(page.ID)); len(files) != 2 {
		t.Errorf("image folder = %v", files)
	}
	items := listOf(t, f, drafthistory.ChatDrafts)
	if len(items) != 2 || items[0].Text != "page brief" || items[0].Name != "page-name" || items[0].ImageCount != 1 {
		t.Errorf("list = %+v", items)
	}

	var stale ops.ChatDraftPickResult
	postPick(t, f, "/v1/agents/drafts/pick-history", ops.DraftHistoryPickRequest{EntryID: entry, DraftID: page.ID, Revision: page.Revision}, &stale)
	if !stale.Refused || stale.Reason != ops.ReasonDraftRevisionConflict {
		t.Errorf("stale = %+v", stale)
	}
	var same ops.ChatDraftPickResult
	postPick(t, f, "/v1/agents/drafts/pick-history", ops.DraftHistoryPickRequest{EntryID: entry, DraftID: page.ID, Revision: got.Revision}, &same)
	if same.Draft == nil || same.Draft.Revision != got.Revision || len(listOf(t, f, drafthistory.ChatDrafts)) != 2 {
		t.Errorf("equal pick changed something: %+v", same)
	}
}
