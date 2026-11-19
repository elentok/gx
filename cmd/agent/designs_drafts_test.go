package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

func TestDesignsDrafts_ListShowDelete(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "a-slug", "the idea\nsecond line")
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

func TestDesignsDrafts_DeleteSavesAHistoryEntry(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "a-slug", "the idea")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var out bytes.Buffer
	if err := runDraftsDelete(ctx, h.Client, &out, false, d.ID); err != nil {
		t.Fatal(err)
	}
	items, err := drafthistory.New(h.StateDir).List(drafthistory.DesignDrafts)
	if err != nil || len(items) != 1 {
		t.Fatalf("entries = %v, %v; want 1", items, err)
	}
	if e := items[0]; e.Text != "the idea" || e.Project != "proj" || e.Slug != "a-slug" {
		t.Errorf("entry = %+v", e)
	}
	if err := runDraftsDelete(ctx, h.Client, &out, false, d.ID); err != nil {
		t.Errorf("delete of an unknown id: %v", err)
	}
	if items, _ := drafthistory.New(h.StateDir).List(drafthistory.DesignDrafts); len(items) != 1 {
		t.Errorf("entries after unknown-id delete = %d, want 1", len(items))
	}
}

func TestDesignsDrafts_RemoveIfEmptyAndLease(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	empty, err := drafts.New(h.StateDir).Create("proj", "", "")
	if err != nil {
		t.Fatal(err)
	}
	full, err := drafts.New(h.StateDir).Create("proj", "", "words")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var out bytes.Buffer

	if err := runDraftsLease(ctx, h.Client, &out, false, empty.ID); err != nil {
		t.Fatalf("lease: %v", err)
	}
	if err := runDraftsRemoveIfEmpty(ctx, h.Client, &out, false, full.ID); err == nil || !strings.Contains(err.Error(), ops.ReasonDraftNotEmpty) {
		t.Errorf("remove-if-empty on a full draft: %v, want a draft-not-empty refusal", err)
	}
	if err := runDraftsRemoveIfEmpty(ctx, h.Client, &out, false, empty.ID); err != nil {
		t.Fatalf("remove-if-empty: %v", err)
	}
	if err := runDraftsLease(ctx, h.Client, &out, false, empty.ID); err == nil || !strings.Contains(err.Error(), ops.ReasonDraftGone) {
		t.Errorf("lease after removal: %v, want a draft-gone refusal", err)
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
	d, err := st.Create("proj", "s", "idea")
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
	d, err := st.Create("proj", "s", "idea")
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

func TestDesignsDrafts_SaveWithWorkspaceCreatesIntoWorkspace(t *testing.T) {
	if newDraftsSaveCmd(Deps{}).Flags().Lookup("workspace") != nil {
		t.Fatal("drafts save has a --workspace flag")
	}
	h := servertest.StartWithStore(t, t.TempDir())
	ctx := context.Background()
	if _, err := h.Client.Admin.CreateWorkspace(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runDraftsSave(ctx, h.Client, &out, true, ops.DesignDraftSaveRequest{Idea: "idea", Workspace: "A"}); err != nil {
		t.Fatal(err)
	}
	var res ops.DesignDraftSaveResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Draft == nil {
		t.Fatalf("save output = %q, %v", out.String(), err)
	}
	if got := h.Server.Workspaces().Workspace(drafts.MemberKey(res.Draft.ID)); got != "A" {
		t.Errorf("created draft workspace = %q, want A", got)
	}
}

func draftsCreateArgs(extra ...string) []string {
	return append([]string{"designs", "drafts", "create", "--idea", "-"}, extra...)
}

func TestDesignsDrafts_CreateReadsIdeaFromStdinInSourceWorkspace(t *testing.T) {
	f := rigs.StartDesigns(t)
	chat := chatIn(t, f, "A")
	t.Setenv(holder.AgentSessionEnv, chat)
	var out bytes.Buffer
	if err := execute(t, draftsCreateArgs("--project", "proj", "--slug", "a-slug"), draftsCmdDeps(f, "the idea\n", &out)); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	d, err := drafts.New(f.H.StateDir).Get(id)
	if err != nil || d == nil || d.Project != "proj" || d.Slug != "a-slug" || strings.TrimSpace(d.Idea) != "the idea" {
		t.Fatalf("draft %q = %+v, %v", id, d, err)
	}
	if got := f.H.Server.Workspaces().Workspace(drafts.MemberKey(id)); got != "A" {
		t.Errorf("draft workspace = %q, want A", got)
	}
}

func TestDesignsDrafts_CreateJSONHasID(t *testing.T) {
	f := rigs.StartDesigns(t)
	t.Setenv(holder.AgentSessionEnv, "")
	var out bytes.Buffer
	if err := execute(t, draftsCreateArgs("--project", "proj", "--slug", "s", "--json"), draftsCmdDeps(f, "idea", &out)); err != nil {
		t.Fatal(err)
	}
	var res ops.DesignDraftSaveResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Draft == nil || res.Draft.ID == "" {
		t.Fatalf("--json output = %q, %v", out.String(), err)
	}
	if got := f.H.Server.Workspaces().Workspace(drafts.MemberKey(res.Draft.ID)); got != workspaces.DefaultName {
		t.Errorf("no session: workspace = %q, want Default", got)
	}
}

func TestDesignsDrafts_CreateRefusals(t *testing.T) {
	t.Setenv(holder.AgentSessionEnv, "")
	big := strings.Repeat("a", wire.MaxSessionPromptBytes+1)
	cases := map[string]struct {
		args  []string
		stdin string
		want  string
	}{
		"too long":   {draftsCreateArgs("--project", "p", "--slug", "s"), big, wire.ReasonPromptTextTooLong},
		"bad slug":   {draftsCreateArgs("--project", "p", "--slug", "Not A Slug"), "idea", "refused"},
		"no slug":    {draftsCreateArgs("--project", "p"), "idea", "slug"},
		"no project": {draftsCreateArgs("--slug", "s"), "idea", "project"},
	}
	for name, c := range cases {
		f := rigs.StartDesigns(t)
		var out bytes.Buffer
		err := execute(t, c.args, draftsCmdDeps(f, c.stdin, &out))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
		if items, _ := f.H.Client.Sessions.DesignDrafts(context.Background()); len(items) != 0 {
			t.Errorf("%s: stored %d drafts", name, len(items))
		}
	}
}

func TestDesignsDrafts_SaveRevisionConflict(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "s", "idea")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	var out bytes.Buffer
	req := ops.DesignDraftSaveRequest{ID: d.ID, Idea: "new", Revision: d.Revision + 3}
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

func TestDesignsDrafts_SetWritesSlugAndProjectWithoutBumpingRevision(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	d, err := drafts.New(h.StateDir).Create("proj", "s", "idea")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	var out bytes.Buffer
	req := ops.DesignDraftTargetRequest{ID: d.ID, Project: new("other"), Slug: new("new-slug")}
	if err := runDraftsSet(context.Background(), h.Client, &out, false, req); err != nil || !strings.Contains(out.String(), "new-slug") {
		t.Fatalf("set = %q, %v", out.String(), err)
	}
	got, err := drafts.New(h.StateDir).Get(d.ID)
	if err != nil || got.Slug != "new-slug" || got.Project != "other" || got.Revision != d.Revision {
		t.Fatalf("draft = %+v, %v", got, err)
	}
	badReq := ops.DesignDraftTargetRequest{ID: d.ID, Slug: new("Bad Slug")}
	if err := runDraftsSet(context.Background(), h.Client, &out, false, badReq); err == nil || !strings.Contains(err.Error(), "is not a slug") {
		t.Errorf("bad slug: err = %v, want the slug refusal", err)
	}
	req.ID = "0123456789abcdef"
	if err := runDraftsSet(context.Background(), h.Client, &out, false, req); err == nil || !strings.Contains(err.Error(), ops.ReasonDraftGone) {
		t.Errorf("missing draft: err = %v, want draft-gone", err)
	}
}
