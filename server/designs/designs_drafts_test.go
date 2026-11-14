package designs_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/designs"
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

// seedDraft writes a draft straight into the store the server reads.
func (f designsFixture) seedDraft(t *testing.T, project, slug, idea string) *ops.DesignDraft {
	t.Helper()
	d, err := drafts.New(f.h.StateDir).Create(project, slug, idea)
	if err != nil || d == nil {
		t.Fatalf("seed draft = %+v, %v", d, err)
	}
	return d
}

func (f designsFixture) webGet(t *testing.T, path string, out any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.ck)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, resp.StatusCode, b)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("GET %s body %s: %v", path, b, err)
	}
}

func (f designsFixture) webSave(t *testing.T, id string, rev int, idea string) ops.DesignDraftSaveResult {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"idea":%q,"revision":%d}`, id, idea, rev)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("save = %d %s", code, out)
	}
	var res ops.DesignDraftSaveResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

// settle waits until every event published so far has been streamed, by
// creating a workspace (the events are ordered) and waiting for its event.
func (f designsFixture) settle(t *testing.T) {
	t.Helper()
	name := fmt.Sprintf("sentinel-%d", time.Now().UnixNano())
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, name); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(f.ctx, t, f.evs, wire.EventWorkspacesChanged, "")
}

// draftEvents runs act and counts the design-drafts-changed events it caused.
// act must not publish workspaces-changed itself.
func (f designsFixture) draftEvents(t *testing.T, act func()) int {
	t.Helper()
	f.settle(t)
	act()
	name := fmt.Sprintf("sentinel-%d", time.Now().UnixNano())
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, name); err != nil {
		t.Fatal(err)
	}
	n := 0
	for {
		select {
		case ev := <-f.evs:
			switch ev.Type {
			case wire.EventDesignDraftsChanged:
				n++
			case wire.EventWorkspacesChanged:
				return n
			}
		case <-f.ctx.Done():
			t.Fatal(f.ctx.Err())
		}
	}
}

func (f designsFixture) wantDraftEvents(t *testing.T, what string, want int, act func()) {
	t.Helper()
	if got := f.draftEvents(t, act); got != want {
		t.Errorf("%s: %d design-drafts-changed events, want %d", what, got, want)
	}
}

func TestWebDesignDrafts_Snapshot(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	if _, err := f.h.Client.Admin.CreateWorkspace(f.ctx, "A"); err != nil {
		t.Fatal(err)
	}
	slugged := f.seedDraft(t, "proj", "my-slug", "secret prompt text")
	prompted := f.seedDraft(t, "proj", "", "one two three four five six")
	if err := f.h.Server.Workspaces().SetMember(drafts.MemberKey(prompted.ID), "A"); err != nil {
		t.Fatal(err)
	}
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]wire.DesignDraftRow{}
	for _, r := range snap.DesignDrafts {
		rows[r.ID] = r
	}
	if r := rows[slugged.ID]; r.Label != "my-slug" || r.Slug != "my-slug" || r.Project != "proj" || r.Workspace != workspaces.DefaultName || r.EditedAt.IsZero() {
		t.Errorf("slug row = %+v", r)
	}
	if r := rows[prompted.ID]; r.Label != "one two three four five" || r.Workspace != "A" {
		t.Errorf("prompt row = %+v", r)
	}
	b, _ := json.Marshal(snap.DesignDrafts)
	if strings.Contains(string(b), `"idea"`) || strings.Contains(string(b), "secret") || strings.Contains(string(b), `"images"`) {
		t.Errorf("rows leak the prompt or images: %s", b)
	}
}

func TestWebDesignDrafts_Event(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	ctx := f.ctx
	var created *ops.DesignDraft
	f.wantDraftEvents(t, "create", 1, func() {
		res, err := f.h.Client.Sessions.SaveDesignDraft(ctx, ops.DesignDraftSaveRequest{Idea: "hello"})
		if err != nil {
			t.Fatal(err)
		}
		created = res.Draft
	})
	target := func(project, slug string) func() {
		return func() {
			if _, err := f.h.Client.Sessions.SetDesignDraftTarget(ctx, ops.DesignDraftTargetRequest{ID: created.ID, Project: &project, Slug: &slug}); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.wantDraftEvents(t, "slug change", 1, target("proj", "a-slug"))
	f.wantDraftEvents(t, "project change", 1, target("other", "a-slug"))
	f.wantDraftEvents(t, "no-op target", 0, target("other", "a-slug"))

	other := f.seedDraft(t, "proj", "gone", "text")
	f.wantDraftEvents(t, "remove", 1, func() {
		if _, err := f.h.Client.Sessions.DeleteDesignDraft(ctx, other.ID); err != nil {
			t.Fatal(err)
		}
	})
	empty := f.seedDraft(t, "", "", "")
	f.wantDraftEvents(t, "remove-if-empty", 1, func() {
		if res, err := f.h.Client.Sessions.RemoveDesignDraftIfEmpty(ctx, empty.ID); err != nil || !res.Deleted {
			t.Fatalf("remove-if-empty = %+v, %v", res, err)
		}
	})
	f.seedDraft(t, "", "", "")
	f.seedDraft(t, "", "", "")
	f.wantDraftEvents(t, "create with a sweep of two", 1, func() {
		if _, err := f.h.Client.Sessions.SaveDesignDraft(ctx, ops.DesignDraftSaveRequest{Idea: "again"}); err != nil {
			t.Fatal(err)
		}
	})
	start := f.seedDraft(t, "proj", "fresh-start", "an idea")
	f.wantDraftEvents(t, "start", 1, func() { f.openFromDraft(t, "fresh-start", "an idea", start.ID) })
}

func TestWebDesignDrafts_EventSilentAutosave(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	ctx := f.ctx
	d := f.seedDraft(t, "proj", "", "one two three four five")
	rev := d.Revision
	save := func(id string, idea string) func() {
		return func() {
			res, err := f.h.Client.Sessions.SaveDesignDraft(ctx, ops.DesignDraftSaveRequest{ID: id, Idea: idea, Revision: rev})
			if err != nil || res.Draft == nil {
				t.Fatalf("save = %+v, %v", res, err)
			}
			rev = res.Draft.Revision
		}
	}
	f.wantDraftEvents(t, "later words", 0, save(d.ID, "one two three four five six seven"))
	f.wantDraftEvents(t, "label change", 1, save(d.ID, "zero two three four five six"))

	slugged := f.seedDraft(t, "proj", "has-slug", "words")
	rev = slugged.Revision
	f.wantDraftEvents(t, "slug-labelled prompt save", 0, save(slugged.ID, "other words entirely"))

	f.wantDraftEvents(t, "image add and remove", 0, func() {
		img, err := f.h.Client.Sessions.AddDesignDraftImage(ctx, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))
		if err != nil || img.ID == "" {
			t.Fatalf("add image = %+v, %v", img, err)
		}
		if _, err := f.h.Client.Sessions.RemoveDesignDraftImage(ctx, d.ID, img.ID); err != nil {
			t.Fatal(err)
		}
	})
	f.wantDraftEvents(t, "lease ping", 0, func() {
		if _, err := f.h.Client.Sessions.LeaseDesignDraft(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	})
	f.wantDraftEvents(t, "last project", 0, func() {
		if _, err := f.h.Client.Sessions.SetDesignLastProject(ctx, "proj"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWebDesignDrafts_ListReadSaveDelete(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	older := f.seedDraft(t, "proj", "older", "first idea")
	for !time.Now().After(older.EditedAt) {
	}
	newer := f.seedDraft(t, "proj", "newer", "second idea")

	var items []ops.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	if len(items) != 2 || items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("list = %+v, want newest first", items)
	}

	var read ops.DesignDraftSaveResult
	f.webGet(t, "/v1/designs/drafts/read?id="+url.QueryEscape(older.ID), &read)
	if read.Refused || read.Draft == nil || read.Draft.Idea != "first idea" {
		t.Fatalf("read = %+v", read)
	}

	saved := f.webSave(t, older.ID, older.Revision, "edited idea")
	if saved.Refused || saved.Draft == nil || saved.Draft.Revision != older.Revision+1 || saved.Draft.Idea != "edited idea" {
		t.Fatalf("save = %+v", saved)
	}

	body := fmt.Sprintf(`{"id":%q}`, older.ID)
	if code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/delete", body, f.ck, "", servertest.JSONPost(f.base)); code != http.StatusOK || servertest.ReasonOf(t, out) != "" {
		t.Fatalf("delete = %d %s", code, out)
	}
	f.webGet(t, "/v1/designs/drafts/read?id="+url.QueryEscape(older.ID), &read)
	if !read.Refused || read.Reason != ops.ReasonDraftGone {
		t.Errorf("read after delete = %+v, want draft-gone", read)
	}
}

func TestWebDesignDrafts_LastProject(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	set := func(p string) {
		t.Helper()
		body := fmt.Sprintf(`{"project":%q}`, p)
		if code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/last-project", body, f.ck, "", servertest.JSONPost(f.base)); code != http.StatusOK {
			t.Fatalf("set = %d %s", code, out)
		}
	}
	read := func() string {
		t.Helper()
		var res ops.DesignLastProject
		f.webGet(t, "/v1/designs/drafts/last-project", &res)
		return res.Project
	}

	if got := read(); got != "" {
		t.Fatalf("fresh server last project = %q, want empty", got)
	}
	set("alpha")
	if got := read(); got != "alpha" {
		t.Fatalf("after set = %q, want alpha", got)
	}
	set("beta")
	if got := read(); got != "beta" {
		t.Fatalf("after second set = %q, want beta", got)
	}

	d := f.seedDraft(t, "beta", "s", "idea")
	body := fmt.Sprintf(`{"id":%q}`, d.ID)
	if code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/delete", body, f.ck, "", servertest.JSONPost(f.base)); code != http.StatusOK {
		t.Fatalf("delete = %d %s", code, out)
	}
	if got := read(); got != "beta" {
		t.Errorf("after draft delete = %q, want beta kept", got)
	}
}

func TestWebDesignDrafts_SaveRefusals(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")

	if res := f.webSave(t, d.ID, d.Revision+5, "x"); !res.Refused || res.Reason != ops.ReasonDraftRevisionConflict {
		t.Errorf("stale save = %+v, want revision-conflict", res)
	}
	if res := f.webSave(t, "0123456789abcdef", 1, "x"); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("save of a missing draft = %+v, want draft-gone", res)
	}

	// The socket (CLI) call refuses the same way as the web call.
	cli, err := f.h.Client.Sessions.SaveDesignDraft(f.ctx, ops.DesignDraftSaveRequest{ID: d.ID, Idea: "x", Revision: d.Revision + 5})
	if err != nil || !cli.Refused || cli.Reason != ops.ReasonDraftRevisionConflict {
		t.Errorf("socket stale save = %+v, %v", cli, err)
	}
	cli, err = f.h.Client.Sessions.SaveDesignDraft(f.ctx, ops.DesignDraftSaveRequest{ID: "0123456789abcdef", Idea: "x", Revision: 1})
	if err != nil || !cli.Refused || cli.Reason != ops.ReasonDraftGone {
		t.Errorf("socket gone save = %+v, %v", cli, err)
	}
}

func TestWebDesignDrafts_SaveWithoutIDCreates(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)

	res := f.webSave(t, "", 0, "first words")
	if res.Refused || res.Draft == nil || res.Draft.ID == "" || res.Draft.Idea != "first words" || res.Draft.Revision != 1 {
		t.Fatalf("create save = %+v", res)
	}
	var items []ops.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	if len(items) != 1 || items[0].ID != res.Draft.ID || items[0].Label != "first words" || res.Draft.Label != "first words" {
		t.Errorf("list = %+v, want the created draft with its label", items)
	}

	var empty ops.DesignDraftSaveResult
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", `{"project":"proj"}`, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK || json.Unmarshal([]byte(out), &empty) != nil || empty.Draft == nil || empty.Draft.Label != "Untitled design" {
		t.Fatalf("empty create = %d %s, want a kept draft", code, out)
	}
	var read ops.DesignDraftSaveResult
	f.webGet(t, "/v1/designs/drafts/read?id="+url.QueryEscape(empty.Draft.ID), &read)
	if read.Draft == nil || read.Draft.Label != "Untitled design" {
		t.Errorf("read = %+v, want the label", read)
	}
}

func (f designsFixture) webTarget(t *testing.T, id, project, slug string) ops.DesignDraftSaveResult {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"project":%q,"slug":%q}`, id, project, slug)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/target", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("target = %d %s", code, out)
	}
	var res ops.DesignDraftSaveResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebDesignDrafts_TargetWriteIsRevisionFree(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "old", "idea")

	res := f.webTarget(t, d.ID, "other", "new")
	if res.Refused || res.Draft == nil || res.Draft.Slug != "new" || res.Draft.Project != "other" || res.Draft.Revision != d.Revision {
		t.Fatalf("target = %+v, want new slug and project at revision %d", res, d.Revision)
	}
	var last ops.DesignLastProject
	f.webGet(t, "/v1/designs/drafts/last-project", &last)
	if last.Project != "other" {
		t.Errorf("last project = %q, want other", last.Project)
	}

	// A text save at the unchanged revision keeps the slug and project.
	saved := f.webSave(t, d.ID, d.Revision, "edited")
	if saved.Refused || saved.Draft == nil || saved.Draft.Slug != "new" || saved.Draft.Project != "other" || saved.Draft.Idea != "edited" {
		t.Fatalf("save after target = %+v", saved)
	}

	if res := f.webTarget(t, d.ID, "other", ""); res.Draft == nil || res.Draft.Slug != "" {
		t.Errorf("empty slug = %+v, want it cleared", res)
	}
	if res := f.webTarget(t, "0123456789abcdef", "p", "s"); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("target of a missing draft = %+v, want draft-gone", res)
	}
}

func TestWebDesignDrafts_TargetRefusesBadSlugAndWritesOnlyPresentFields(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "old", "idea")
	post := func(body string) ops.DesignDraftSaveResult {
		t.Helper()
		code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/target", body, f.ck, "", servertest.JSONPost(f.base))
		var res ops.DesignDraftSaveResult
		if code != http.StatusOK || json.Unmarshal([]byte(out), &res) != nil {
			t.Fatalf("target = %d %s", code, out)
		}
		return res
	}
	bad := post(fmt.Sprintf(`{"id":%q,"slug":"Bad Slug"}`, d.ID))
	if !bad.Refused || !strings.Contains(bad.Reason, "is not a slug") {
		t.Errorf("bad slug = %+v, want the slug refusal", bad)
	}
	if res := post(fmt.Sprintf(`{"id":%q,"project":"other"}`, d.ID)); res.Draft == nil || res.Draft.Project != "other" || res.Draft.Slug != "old" {
		t.Errorf("project only = %+v, want slug kept", res)
	}
}

func TestWebDesignDrafts_NameMigrationRunsAtStart(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	d, err := drafts.New(state).Create("proj", "", "")
	if err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(state, "design-drafts", d.ID, "draft.json")
	old := fmt.Sprintf(`{"id":%q,"project":"proj","slug":"","name":"My Cool Idea","idea":"","images":[],"revision":1,"edited_at":"2026-01-01T00:00:00Z"}`, d.ID)
	if err := os.WriteFile(rec, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	designs.New(designs.Deps{StateDir: state})

	got, err := drafts.New(state).Get(d.ID)
	if err != nil || got.Slug != "my-cool-idea" {
		t.Fatalf("draft after start = %+v, %v; want the name as slug", got, err)
	}
	if b, _ := os.ReadFile(rec); strings.Contains(string(b), `"name"`) {
		t.Errorf("record still has a name key: %s", b)
	}
}

func TestWebDesignDrafts_EmptyingSaveKeeps(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, err := drafts.New(f.h.StateDir).Create("", "", "only the idea")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	body := fmt.Sprintf(`{"id":%q,"revision":%d}`, d.ID, d.Revision)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK || strings.Contains(out, `"deleted"`) || !strings.Contains(out, `"label":"Untitled design"`) {
		t.Fatalf("emptying save = %d %s", code, out)
	}
	var items []ops.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	if len(items) != 1 || items[0].Label != "Untitled design" {
		t.Errorf("list = %+v, want the kept draft", items)
	}
}

func (f designsFixture) webPostDraft(t *testing.T, route, id string) ops.DesignDraftSaveResult {
	t.Helper()
	code, out := servertest.WebPostJSON(t, f.base+route, fmt.Sprintf(`{"id":%q}`, id), f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("%s = %d %s", route, code, out)
	}
	var res ops.DesignDraftSaveResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebDesignDrafts_RemoveIfEmpty(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	empty := f.seedDraft(t, "proj", "", "")
	full := f.seedDraft(t, "proj", "", "words")

	if res := f.webPostDraft(t, "/v1/designs/drafts/remove-if-empty", empty.ID); !res.Deleted {
		t.Errorf("empty draft: %+v, want deleted", res)
	}
	if res := f.webPostDraft(t, "/v1/designs/drafts/remove-if-empty", full.ID); res.Deleted || res.Reason != ops.ReasonDraftNotEmpty {
		t.Errorf("draft with a prompt: %+v, want draft-not-empty", res)
	}
	if res := f.webPostDraft(t, "/v1/designs/drafts/remove-if-empty", empty.ID); res.Reason != ops.ReasonDraftGone {
		t.Errorf("already gone: %+v, want draft-gone", res)
	}
	var read ops.DesignDraftSaveResult
	f.webGet(t, "/v1/designs/drafts/read?id="+empty.ID, &read)
	if !read.Refused || read.Reason != ops.ReasonDraftGone {
		t.Errorf("read of a removed draft = %+v, want draft-gone", read)
	}
}

func TestWebDesignDrafts_Lease(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "", "")
	if res := f.webPostDraft(t, "/v1/designs/drafts/lease", d.ID); res.Refused {
		t.Errorf("ping = %+v, want accepted", res)
	}
	if res := f.webPostDraft(t, "/v1/designs/drafts/lease", "0123456789abcdef"); res.Reason != ops.ReasonDraftGone {
		t.Errorf("ping on a missing draft = %+v, want draft-gone", res)
	}
}

func TestWebDesignDrafts_SweepOnCreateAndRestart(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	old := f.seedDraft(t, "proj", "", "")
	leased := f.seedDraft(t, "proj", "", "")
	full := f.seedDraft(t, "proj", "", "words")
	f.webPostDraft(t, "/v1/designs/drafts/lease", leased.ID)

	created := f.webSave(t, "", 0, "")
	if created.Draft == nil {
		t.Fatalf("create = %+v", created)
	}
	var items []ops.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	got := map[string]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if got[old.ID] || !got[leased.ID] || !got[full.ID] || !got[created.Draft.ID] {
		t.Errorf("after create: %v; want the old empty draft swept, the leased, non-empty and new ones kept", got)
	}

	// A restart has no live lease: the empty drafts go, the others stay.
	designs.New(designs.Deps{StateDir: f.h.StateDir})
	store := drafts.New(f.h.StateDir)
	for _, id := range []string{leased.ID, created.Draft.ID} {
		if _, err := store.Get(id); err != drafts.ErrNotFound {
			t.Errorf("empty draft %s after restart: %v, want gone", id, err)
		}
	}
	if _, err := store.Get(full.ID); err != nil {
		t.Errorf("non-empty draft after restart: %v", err)
	}
}

func TestWebDesignDrafts_RestartSweepKeepsAMigratedNameOnlyDraft(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	d, err := drafts.New(state).Create("proj", "", "")
	if err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(state, "design-drafts", d.ID, "draft.json")
	old := fmt.Sprintf(`{"id":%q,"project":"proj","slug":"","name":"Named","idea":"","images":[],"revision":1,"edited_at":"2026-01-01T00:00:00Z"}`, d.ID)
	if err := os.WriteFile(rec, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	designs.New(designs.Deps{StateDir: state})
	if got, err := drafts.New(state).Get(d.ID); err != nil || got.Slug != "named" {
		t.Fatalf("name-only draft after start = %+v, %v; want it kept with its slug", got, err)
	}
}

func TestWebDesignDrafts_DraftsAreNotEpics(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "a-draft", "idea")
	var epics json.RawMessage
	f.webGet(t, "/v1/epics", &epics)
	if s := string(epics); strings.Contains(s, "a-draft") || strings.Contains(s, d.ID) {
		t.Errorf("draft shows in the epic list: %s", s)
	}
}
