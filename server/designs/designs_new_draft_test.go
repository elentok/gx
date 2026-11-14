package designs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

func (f designsFixture) openFromDraft(t *testing.T, slug, idea, draft string) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: slug, NewDesign: true, Idea: idea, Project: "proj", DraftID: draft})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// draftWithImages seeds a draft holding two PNG uploads and returns it with the image file names.
func (f designsFixture) draftWithImages(t *testing.T) (*ops.DesignDraft, []string) {
	t.Helper()
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	var names []string
	for range 2 {
		res := f.uploadDraftImage(t, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))
		if res.Refused {
			t.Fatalf("upload = %+v", res)
		}
		names = append(names, res.Name)
	}
	return d, names
}

func TestDesignsOpenNew_FromDraftMovesImagesAndListsThem(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	imagesDir := filepath.Join(f.h.TicketStore, "proj", "fresh-idea", "images")
	raw, err := os.ReadFile(f.mapPath("fresh-idea"))
	if err != nil {
		t.Fatal(err)
	}
	want := "## Initial prompt\n\nan idea\n\nImages:\n" + filepath.Join(imagesDir, names[0]) + "\n" + filepath.Join(imagesDir, names[1]) + "\n"
	if !strings.Contains(string(raw), want) {
		t.Errorf("map lacks %q:\n%s", want, raw)
	}
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(imagesDir, n)); err != nil {
			t.Errorf("image %s not moved: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(drafts.New(f.h.StateDir).ImagesDir(d.ID))); err == nil {
		t.Error("draft folder still exists")
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "proj" {
		t.Errorf("last project = %q, want proj", got)
	}
	if meta := readMeta(t, f.h, res.Session.ID); meta["first_prompt"] != "/gx-design proj:fresh-idea" {
		t.Errorf("first_prompt = %v, want the bare skill command", meta["first_prompt"])
	}
}

func TestDesignsOpenNew_WithoutImagesAddsNoImagesLine(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	if res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	raw, _ := os.ReadFile(f.mapPath("fresh-idea"))
	if strings.Contains(string(raw), "Images:") || !strings.HasSuffix(string(raw), "## Initial prompt\n\nan idea\n") {
		t.Errorf("map = %s", raw)
	}
}

func TestDesignsOpenNew_MissingDraftStillStarts(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if res := f.openFromDraft(t, "fresh-idea", "an idea", "0123456789abcdef"); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "proj" {
		t.Errorf("last project = %q, want proj", got)
	}
}

func TestDesignsOpenNew_EpicExistsKeepsTheDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	res := f.openFromDraft(t, "map", "an idea", d.ID)
	if !res.Refused || res.Reason != wire.ReasonDesignEpicExists {
		t.Fatalf("open = %+v", res)
	}
	if got := f.readDraft(t, d.ID); len(got.Images) != len(names) {
		t.Errorf("draft images = %v", got.Images)
	}
	if got, _ := drafts.New(f.h.StateDir).LastProject(); got != "" {
		t.Errorf("last project = %q, want unset", got)
	}
}

func TestDesignsOpenNew_FromDraftStartFailureDeletesTheDraftAndKeepsTheEpic(t *testing.T) {
	t.Parallel()
	f := startDesigns(t, servertest.WithChatHarness("no-such-harness"), servertest.WithDefaultHarness("no-such-harness"))
	d, names := f.draftWithImages(t)
	res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID)
	if res.Refused || !res.EpicCreated || res.StartError == "" || res.Session != nil {
		t.Fatalf("open = %+v, want epic_created with start_error and no refusal", res)
	}
	if _, err := drafts.New(f.h.StateDir).Get(d.ID); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("draft after failed start: err = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(drafts.New(f.h.StateDir).ImagesDir(d.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("draft image folder still there: %v", err)
	}
	if _, err := os.Stat(f.mapPath("fresh-idea")); err != nil {
		t.Errorf("Design epic missing: %v", err)
	}
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(f.h.TicketStore, "proj", "fresh-idea", "images", n)); err != nil {
			t.Errorf("image %s not in the epic: %v", n, err)
		}
	}
}

func TestDesignsOpenNew_MapWriteFailureKeepsTheDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, names := f.draftWithImages(t)
	projDir := filepath.Join(f.h.TicketStore, "proj")
	if err := os.Chmod(projDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projDir, 0o755) })
	if _, err := os.Create(filepath.Join(projDir, "probe")); err == nil {
		t.Skip("directory permissions are not enforced here")
	}
	if _, err := f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: "fresh-idea", NewDesign: true, Idea: "an idea", Project: "proj", DraftID: d.ID}); err == nil {
		t.Fatal("open succeeded, want a failed map write")
	}
	if got := f.readDraft(t, d.ID); len(got.Images) != len(names) {
		t.Errorf("draft images = %v", got.Images)
	}
}

func TestDesignsOpenNew_DraftWorkspaceWins(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	for _, name := range []string{"A", "B"} {
		if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	ws := f.h.Server.Workspaces()
	d := f.seedDraft(t, "proj", "in-a", "an idea")
	if err := ws.SetMember(drafts.MemberKey(d.ID), "A"); err != nil {
		t.Fatal(err)
	}
	res, err := f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: "in-a", NewDesign: true, Idea: "an idea", Project: "proj", DraftID: d.ID, Workspace: "B"})
	if err != nil || res.Refused {
		t.Fatalf("open = %+v, %v", res, err)
	}
	if got := ws.Workspace("proj:in-a"); got != "A" {
		t.Errorf("design workspace = %q, want A", got)
	}
	bare := f.seedDraft(t, "proj", "in-default", "an idea")
	res, err = f.h.Client.Sessions.OpenDesign(f.ctx, wire.DesignOpenRequest{Epic: "in-default", NewDesign: true, Idea: "an idea", Project: "proj", DraftID: bare.ID, Workspace: "B"})
	if err != nil || res.Refused {
		t.Fatalf("open = %+v, %v", res, err)
	}
	if got := ws.Workspace("proj:in-default"); got != workspaces.DefaultName {
		t.Errorf("design workspace = %q, want %q", got, workspaces.DefaultName)
	}
}

// membersFile is the raw text of the workspace store's file under the state dir.
func (f designsFixture) membersFile(t *testing.T) string {
	t.Helper()
	var text string
	err := filepath.WalkDir(f.h.StateDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "workspaces.json" {
			b, rerr := os.ReadFile(p)
			text, err = string(b), rerr
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// draftInWorkspace seeds a draft and moves it to a new workspace "A".
func (f designsFixture) draftInWorkspace(t *testing.T, slug, idea string) *ops.DesignDraft {
	t.Helper()
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, "A"); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	d := f.seedDraft(t, "proj", slug, idea)
	if err := f.h.Server.Workspaces().SetMember(drafts.MemberKey(d.ID), "A"); err != nil {
		t.Fatal(err)
	}
	return d
}

func (f designsFixture) wantNoMembership(t *testing.T, d *ops.DesignDraft) {
	t.Helper()
	key := drafts.MemberKey(d.ID)
	if got := f.h.Server.Workspaces().Workspace(key); got != workspaces.DefaultName {
		t.Errorf("workspace of %s = %q, want Default", key, got)
	}
	if strings.Contains(f.membersFile(t), d.ID) {
		t.Errorf("members file still lists %s", d.ID)
	}
}

func TestDesignsOpenNew_DropsDraftMembership(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.draftInWorkspace(t, "dropped", "an idea")
	f.openFromDraft(t, "dropped", "an idea", d.ID)
	f.wantNoMembership(t, d)
	if got := f.h.Server.Workspaces().Workspace("proj:dropped"); got != "A" {
		t.Errorf("design workspace = %q, want A", got)
	}
}

func TestWebDesignDrafts_DeleteDropsMembership(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	byRoute := f.draftInWorkspace(t, "by-route", "an idea")
	if _, err := f.h.Client.Sessions.DeleteDesignDraft(f.ctx, byRoute.ID); err != nil {
		t.Fatal(err)
	}
	f.wantNoMembership(t, byRoute)

	empty := f.draftInWorkspace(t, "", "")
	res, err := f.h.Client.Sessions.RemoveDesignDraftIfEmpty(f.ctx, empty.ID)
	if err != nil || !res.Deleted {
		t.Fatalf("remove-if-empty = %+v, %v", res, err)
	}
	f.wantNoMembership(t, empty)

	// The sweep that runs on a create removes other empty drafts.
	swept := f.draftInWorkspace(t, "", "")
	if _, err := f.h.Client.Sessions.SaveDesignDraft(f.ctx, ops.DesignDraftSaveRequest{Idea: "new text"}); err != nil {
		t.Fatal(err)
	}
	f.wantNoMembership(t, swept)
}

func TestWebDesignDrafts_CreateJoinsWorkspace(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, "A"); err != nil {
		t.Fatal(err)
	}
	ws := f.h.Server.Workspaces()
	save := func(req ops.DesignDraftSaveRequest) ops.DesignDraftSaveResult {
		t.Helper()
		res, err := f.h.Client.Sessions.SaveDesignDraft(f.ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	joined := save(ops.DesignDraftSaveRequest{Idea: "one", Workspace: "A"})
	if got := ws.Workspace(drafts.MemberKey(joined.Draft.ID)); got != "A" {
		t.Errorf("created draft workspace = %q, want A", got)
	}
	if !strings.Contains(f.membersFile(t), joined.Draft.ID) {
		t.Errorf("members file does not list %s", joined.Draft.ID)
	}
	for _, name := range []string{"", workspaces.DefaultName} {
		d := save(ops.DesignDraftSaveRequest{Idea: "two " + name, Workspace: name}).Draft
		f.wantNoMembership(t, d)
	}

	before, _ := f.h.Client.Sessions.DesignDrafts(f.ctx)
	res := save(ops.DesignDraftSaveRequest{Idea: "three", Workspace: "Nope"})
	after, _ := f.h.Client.Sessions.DesignDrafts(f.ctx)
	if !res.Refused || res.Reason != ops.ReasonDraftUnknownWorkspace || len(after) != len(before) {
		t.Errorf("unknown workspace = %+v, drafts %d -> %d, want refused and none stored", res, len(before), len(after))
	}

	other := f.seedDraft(t, "proj", "existing", "idea")
	save(ops.DesignDraftSaveRequest{ID: other.ID, Idea: "edited", Revision: other.Revision, Workspace: "A"})
	f.wantNoMembership(t, other)
}

func TestDesignsOpenNew_StartRecord(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	if res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID); res.Refused {
		t.Fatalf("open = %+v", res)
	}
	st := drafts.New(f.h.StateDir)
	if got := st.Started(d.ID); got != "proj:fresh-idea" {
		t.Errorf("Started = %q", got)
	}
	// A Start with a missing draft writes no record.
	f.openFromDraft(t, "other-idea", "an idea", "0123456789abcdef")
	if got := st.Started("0123456789abcdef"); got != "" {
		t.Errorf("Started for a missing draft = %q", got)
	}
	// A failed start keeps the draft and writes no record.
	keep := f.seedDraft(t, "proj", "map", "an idea")
	if res := f.openFromDraft(t, "map", "an idea", keep.ID); !res.Refused {
		t.Fatalf("open = %+v, want a refusal", res)
	}
	if got := st.Started(keep.ID); got != "" {
		t.Errorf("Started after a refused start = %q", got)
	}
}

func TestWebDesignDrafts_ReadRedirect(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	f.openFromDraft(t, "fresh-idea", "an idea", d.ID)
	var res ops.DesignDraftSaveResult
	f.webGet(t, "/v1/designs/drafts/read?id="+d.ID, &res)
	if !res.Refused || res.Reason != ops.ReasonDraftGone || res.Started != "proj:fresh-idea" {
		t.Errorf("read started = %+v", res)
	}
	res = ops.DesignDraftSaveResult{}
	f.webGet(t, "/v1/designs/drafts/read?id=0123456789abcdef", &res)
	if !res.Refused || res.Reason != ops.ReasonDraftGone || res.Started != "" {
		t.Errorf("read unknown = %+v", res)
	}
}

func TestWebDesignDrafts_SaveRedirect(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "fresh-idea", "an idea")
	f.openFromDraft(t, "fresh-idea", "an idea", d.ID)
	res := f.webSave(t, d.ID, d.Revision, "edited")
	if !res.Refused || res.Reason != ops.ReasonDraftGone || res.Started != "proj:fresh-idea" {
		t.Errorf("save started = %+v", res)
	}
}
