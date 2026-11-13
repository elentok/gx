package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

func TestDesignsDrafts_ListShowDelete(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "a-slug", "the idea\nsecond line", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	ctx := context.Background()

	var out bytes.Buffer
	if err := runDraftsList(ctx, h.Client, &out, false); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, d.ID) || !strings.Contains(got, "a-slug") {
		t.Errorf("list = %q, want the id and slug", got)
	}

	out.Reset()
	if err := runDraftsList(ctx, h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Drafts []ops.DesignDraftItem `json:"drafts"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil || len(listed.Drafts) != 1 || listed.Drafts[0].ID != d.ID {
		t.Errorf("list --json = %s, %v", out.String(), err)
	}

	out.Reset()
	if err := runDraftsShow(ctx, h.Client, &out, false, d.ID); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "the idea\nsecond line\n" {
		t.Errorf("show = %q, want the idea", got)
	}

	out.Reset()
	if err := runDraftsShow(ctx, h.Client, &out, true, d.ID); err != nil {
		t.Fatal(err)
	}
	var shown ops.DesignDraftSaveResult
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil || shown.Draft == nil || shown.Draft.ID != d.ID {
		t.Errorf("show --json = %s, %v", out.String(), err)
	}

	out.Reset()
	if err := runDraftsDelete(ctx, h.Client, &out, false, d.ID); err != nil || out.Len() != 0 {
		t.Fatalf("delete = %q, %v", out.String(), err)
	}
	err = runDraftsShow(ctx, h.Client, &out, false, d.ID)
	if err == nil || !strings.Contains(err.Error(), ops.ReasonDraftGone) {
		t.Errorf("show after delete: err = %v, want a draft-gone refusal", err)
	}
}

func TestDesignsDrafts_LastProject(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	ctx := context.Background()
	var out bytes.Buffer

	if err := runDraftsLastProject(ctx, h.Client, &out, false, ""); err != nil || out.String() != "\n" {
		t.Fatalf("fresh read = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runDraftsLastProject(ctx, h.Client, &out, false, "alpha"); err != nil || out.String() != "alpha\n" {
		t.Fatalf("set = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runDraftsLastProject(ctx, h.Client, &out, true, ""); err != nil {
		t.Fatal(err)
	}
	var got ops.DesignLastProject
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Project != "alpha" {
		t.Errorf("read --json = %s, %v", out.String(), err)
	}
}

func TestDesignsDrafts_ImageAddRm(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	st := drafts.New(h.StateDir)
	d, err := st.Create("proj", "s", "idea", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(file, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runDraftsImageAdd(ctx, h.Client, &out, false, d.ID, file); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	got, err := st.Get(d.ID)
	if err != nil || len(got.Images) != 1 || got.Images[0] != id {
		t.Fatalf("draft after add = %+v, %v; want image %q", got, err, id)
	}

	if err := runDraftsImageRm(ctx, h.Client, &out, false, d.ID, id); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Get(d.ID); len(got.Images) != 0 {
		t.Errorf("images after rm = %v, want none", got.Images)
	}
	err = runDraftsImageRm(ctx, h.Client, &out, false, d.ID, id)
	if err == nil || !strings.Contains(err.Error(), wire.ReasonImageNotFound) {
		t.Errorf("second rm err = %v, want image-not-found", err)
	}
}

func TestDesignsDrafts_ImageGet(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	st := drafts.New(h.StateDir)
	d, err := st.Create("proj", "s", "idea", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	ctx := context.Background()
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	res, err := h.Client.Sessions.AddDesignDraftImage(ctx, d.ID, png)
	if err != nil || res.Refused {
		t.Fatal(res, err)
	}
	data, ct, err := h.Client.Sessions.DesignDraftImage(ctx, d.ID, res.ID)
	if err != nil || ct != "image/png" || !bytes.Equal(data, png) {
		t.Errorf("DesignDraftImage = %d bytes %q, %v; want the png", len(data), ct, err)
	}
	if _, _, err := h.Client.Sessions.DesignDraftImage(ctx, d.ID, "00000000"); err == nil {
		t.Error("unknown image id read without an error")
	}
}

func TestDesignsImages_ListGet(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "map", "01", "first", servertest.TicketOpts{})
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	imgs := filepath.Join(store, "proj", "map", "images")
	if err := os.MkdirAll(imgs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgs, "img-1-aaaaaaaa.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()

	list, err := h.Client.Sessions.DesignImages(ctx, "proj:map")
	if err != nil || len(list) != 1 || list[0].ID != "aaaaaaaa" {
		t.Fatalf("DesignImages = %+v, %v; want aaaaaaaa", list, err)
	}
	data, ct, err := h.Client.Sessions.DesignImage(ctx, "proj:map", "aaaaaaaa")
	if err != nil || ct != "image/png" || !bytes.Equal(data, png) {
		t.Errorf("DesignImage = %d bytes %q, %v; want the png", len(data), ct, err)
	}
	if _, _, err := h.Client.Sessions.DesignImage(ctx, "proj:map", "ticket.md"); err == nil {
		t.Error("ticket.md read without an error")
	}
}

func TestDesignsDrafts_SaveRevisionConflict(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "s", "idea", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	var out bytes.Buffer
	req := ops.DesignDraftSaveRequest{ID: d.ID, Project: "proj", Idea: "new", Revision: d.Revision + 3}
	err = runDraftsSave(context.Background(), h.Client, &out, false, req)
	if err == nil || !strings.Contains(err.Error(), ops.ReasonDraftRevisionConflict) {
		t.Errorf("stale save: err = %v, want revision-conflict", err)
	}
	req.Revision = d.Revision
	out.Reset()
	if err := runDraftsSave(context.Background(), h.Client, &out, false, req); err != nil || !strings.Contains(out.String(), "revision 2") {
		t.Errorf("save = %q, %v", out.String(), err)
	}
}
