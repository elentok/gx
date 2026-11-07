package designs_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// seedDraft writes a draft straight into the store the server reads.
func (f designsFixture) seedDraft(t *testing.T, project, slug, idea string) *wire.DesignDraft {
	t.Helper()
	d, err := drafts.New(f.h.StateDir).Create(project, slug, idea, "")
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

func (f designsFixture) webSave(t *testing.T, id string, rev int, idea string) wire.DesignDraftSaveResult {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"project":"proj","slug":"s","idea":%q,"revision":%d}`, id, idea, rev)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("save = %d %s", code, out)
	}
	var res wire.DesignDraftSaveResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebDesignDrafts_ListReadSaveDelete(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	older := f.seedDraft(t, "proj", "older", "first idea")
	time.Sleep(10 * time.Millisecond)
	newer := f.seedDraft(t, "proj", "newer", "second idea")

	var items []wire.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	if len(items) != 2 || items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("list = %+v, want newest first", items)
	}

	var read wire.DesignDraftSaveResult
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
	if !read.Refused || read.Reason != wire.ReasonDraftGone {
		t.Errorf("read after delete = %+v, want draft-gone", read)
	}
}

func TestWebDesignDrafts_SaveRefusals(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")

	if res := f.webSave(t, d.ID, d.Revision+5, "x"); !res.Refused || res.Reason != wire.ReasonDraftRevisionConflict {
		t.Errorf("stale save = %+v, want revision-conflict", res)
	}
	if res := f.webSave(t, "0123456789abcdef", 1, "x"); !res.Refused || res.Reason != wire.ReasonDraftGone {
		t.Errorf("save of a missing draft = %+v, want draft-gone", res)
	}

	// The socket (CLI) call refuses the same way as the web call.
	cli, err := f.h.Client.Sessions.SaveDesignDraft(f.ctx, wire.DesignDraftSaveRequest{ID: d.ID, Project: "proj", Idea: "x", Revision: d.Revision + 5})
	if err != nil || !cli.Refused || cli.Reason != wire.ReasonDraftRevisionConflict {
		t.Errorf("socket stale save = %+v, %v", cli, err)
	}
	cli, err = f.h.Client.Sessions.SaveDesignDraft(f.ctx, wire.DesignDraftSaveRequest{ID: "0123456789abcdef", Idea: "x", Revision: 1})
	if err != nil || !cli.Refused || cli.Reason != wire.ReasonDraftGone {
		t.Errorf("socket gone save = %+v, %v", cli, err)
	}
}

func TestWebDesignDrafts_EmptyingSaveDeletes(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d, err := drafts.New(f.h.StateDir).Create("", "", "only the idea", "")
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	body := fmt.Sprintf(`{"id":%q,"revision":%d}`, d.ID, d.Revision)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/save", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK || !strings.Contains(out, `"deleted":true`) {
		t.Fatalf("emptying save = %d %s", code, out)
	}
	var items []wire.DesignDraftItem
	f.webGet(t, "/v1/designs/drafts", &items)
	if len(items) != 0 {
		t.Errorf("list = %+v, want none", items)
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
