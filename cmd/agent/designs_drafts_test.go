package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
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
		Drafts []wire.DesignDraftItem `json:"drafts"`
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
	var shown wire.DesignDraftSaveResult
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil || shown.Draft == nil || shown.Draft.ID != d.ID {
		t.Errorf("show --json = %s, %v", out.String(), err)
	}

	out.Reset()
	if err := runDraftsDelete(ctx, h.Client, &out, false, d.ID); err != nil || out.Len() != 0 {
		t.Fatalf("delete = %q, %v", out.String(), err)
	}
	err = runDraftsShow(ctx, h.Client, &out, false, d.ID)
	if err == nil || !strings.Contains(err.Error(), wire.ReasonDraftGone) {
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
	var got wire.DesignLastProject
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Project != "alpha" {
		t.Errorf("read --json = %s, %v", out.String(), err)
	}
}

func TestDesignsDrafts_SaveRevisionConflict(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "s", "idea", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	var out bytes.Buffer
	req := wire.DesignDraftSaveRequest{ID: d.ID, Project: "proj", Idea: "new", Revision: d.Revision + 3}
	err = runDraftsSave(context.Background(), h.Client, &out, false, req)
	if err == nil || !strings.Contains(err.Error(), wire.ReasonDraftRevisionConflict) {
		t.Errorf("stale save: err = %v, want revision-conflict", err)
	}
	req.Revision = d.Revision
	out.Reset()
	if err := runDraftsSave(context.Background(), h.Client, &out, false, req); err != nil || !strings.Contains(out.String(), "revision 2") {
		t.Errorf("save = %q, %v", out.String(), err)
	}
}
