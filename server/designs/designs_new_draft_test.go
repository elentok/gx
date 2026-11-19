package designs_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/drafthistory"
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

func (f designsFixture) designHistory(t *testing.T) []drafthistory.Item {
	t.Helper()
	items, err := drafthistory.New(f.h.StateDir).List(drafthistory.DesignDrafts)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestDesignsOpenNew_StartSavesOneHistoryEntry(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, _ := f.draftWithImages(t)
	if res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID); res.Refused {
		t.Fatalf("open = %+v", res)
	}
	items := f.designHistory(t)
	if len(items) != 1 {
		t.Fatalf("entries = %d, want 1", len(items))
	}
	if e := items[0]; e.Text != "an idea" || e.Project != "proj" || e.Slug != "fresh-idea" || e.ImageCount != 2 {
		t.Errorf("entry = %+v", e)
	}
}

func TestDesignsOpenNew_StartSavesTheRequestsSlugAndIdeaNotTheStoredDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "old-slug", "old text")
	if res := f.openFromDraft(t, "fresh-idea", "newer text", d.ID); res.Refused {
		t.Fatalf("open = %+v", res)
	}
	items := f.designHistory(t)
	if len(items) != 1 {
		t.Fatalf("entries = %d, want 1", len(items))
	}
	if e := items[0]; e.Text != "newer text" || e.Slug != "fresh-idea" || e.Project != "proj" {
		t.Errorf("entry = %+v", e)
	}
}

func TestDesignsOpenNew_GoneDraftSavesNothingAndStartGoesOn(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if res := f.openFromDraft(t, "fresh-idea", "an idea", "0123456789abcdef"); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if n := len(f.designHistory(t)); n != 0 {
		t.Errorf("entries = %d, want 0", n)
	}
}

func TestDesignsOpenNew_FailedStartKeepsEntryAndRetryAddsNone(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, _ := f.draftWithImages(t)
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
	if n := len(f.designHistory(t)); n != 1 {
		t.Fatalf("entries after failed start = %d, want 1", n)
	}
	if err := os.Chmod(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if res := f.openFromDraft(t, "fresh-idea", "an idea", d.ID); res.Refused {
		t.Fatalf("retry = %+v", res)
	}
	if n := len(f.designHistory(t)); n != 1 {
		t.Errorf("entries after retry = %d, want 1", n)
	}
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

func (f designsFixture) createDraft(t *testing.T, req ops.DesignDraftCreateRequest) ops.DesignDraftSaveResult {
	t.Helper()
	res, err := f.h.Client.Sessions.CreateDesignDraft(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebDesignDrafts_CreateRouteJoinsSourceWorkspace(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, "A"); err != nil {
		t.Fatal(err)
	}
	chat, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir()})
	if err != nil || chat.Session == nil {
		t.Fatalf("create chat = %+v, %v", chat, err)
	}
	if res, err := f.h.Client.Admin.MoveToWorkspace(f.ctx, "A", chat.Session.ID); err != nil || res.Results[0].Refused() {
		t.Fatalf("move chat = %+v, %v", res, err)
	}
	design := f.openNew(t, "source-design", "idea")
	if res, err := f.h.Client.Admin.MoveToWorkspace(f.ctx, "A", "proj:source-design"); err != nil || res.Results[0].Refused() {
		t.Fatalf("move design = %+v, %v", res, err)
	}
	ws := f.h.Server.Workspaces()
	for name, from := range map[string]string{"chat": chat.Session.ID, "design": design.Session.ID} {
		res := f.createDraft(t, ops.DesignDraftCreateRequest{Project: "proj", Slug: "from-" + name, Idea: "the idea", FromSession: from})
		if res.Refused || res.Draft == nil || res.Draft.Project != "proj" || res.Draft.Slug != "from-"+name || res.Draft.Idea != "the idea" {
			t.Fatalf("%s: create = %+v", name, res)
		}
		if got := ws.Workspace(drafts.MemberKey(res.Draft.ID)); got != "A" {
			t.Errorf("%s: draft workspace = %q, want A", name, got)
		}
	}
	for _, from := range []string{"", "no-such-session"} {
		d := f.createDraft(t, ops.DesignDraftCreateRequest{Project: "proj", Slug: "stays-default", Idea: "x", FromSession: from}).Draft
		f.wantNoMembership(t, d)
	}
}

func TestWebDesignDrafts_CreateRoutePublishesDraftEvent(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	f.wantDraftEvents(t, "create", 1, func() {
		f.createDraft(t, ops.DesignDraftCreateRequest{Project: "proj", Slug: "evented", Idea: "idea"})
	})
}

func TestWebDesignDrafts_CreateRouteRefusals(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	cases := map[string]struct {
		req    ops.DesignDraftCreateRequest
		reason string
	}{
		"too long":      {ops.DesignDraftCreateRequest{Project: "p", Slug: "s", Idea: strings.Repeat("a", wire.MaxSessionPromptBytes+1)}, wire.ReasonPromptTextTooLong},
		"empty idea":    {ops.DesignDraftCreateRequest{Project: "p", Slug: "s", Idea: " \n"}, "idea"},
		"blank project": {ops.DesignDraftCreateRequest{Project: " ", Slug: "s", Idea: "i"}, "project"},
		"bad slug":      {ops.DesignDraftCreateRequest{Project: "p", Slug: "Bad Slug", Idea: "i"}, "not a slug"},
		"missing slug":  {ops.DesignDraftCreateRequest{Project: "p", Idea: "i"}, "not a slug"},
	}
	for name, c := range cases {
		res := f.createDraft(t, c.req)
		if !res.Refused || res.Draft != nil || !strings.Contains(res.Reason, c.reason) {
			t.Errorf("%s: result = %+v, want refused with %q", name, res, c.reason)
		}
	}
	if items, err := f.h.Client.Sessions.DesignDrafts(f.ctx); err != nil || len(items) != 0 {
		t.Errorf("drafts after refusals = %v, %v, want none", items, err)
	}
}

func TestDesignsOpenNew_CreatedDraftIdeaBecomesInitialPrompt(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.createDraft(t, ops.DesignDraftCreateRequest{Project: "proj", Slug: "made-by-verb", Idea: "stored idea"}).Draft
	if res := f.openFromDraft(t, "made-by-verb", "stored idea", d.ID); res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	got, err := os.ReadFile(f.mapPath("made-by-verb"))
	if err != nil || !strings.Contains(string(got), "## Initial prompt\n\nstored idea") {
		t.Errorf("map = %q, %v", got, err)
	}
}
