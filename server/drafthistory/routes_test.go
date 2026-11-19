package drafthistory_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire/ops"
)

func listRoute(t *testing.T, f rigs.DesignsFixture, list string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(ops.DraftHistoryListRequest{List: list})
	return servertest.WebPostJSON(t, f.Base+"/v1/draft-history/list", string(body), f.Ck, "", servertest.JSONPost(f.Base))
}

func listOK(t *testing.T, f rigs.DesignsFixture, list string) ops.DraftHistoryListResult {
	t.Helper()
	code, out := listRoute(t, f, list)
	if code != http.StatusOK {
		t.Fatalf("list %s = %d %s", list, code, out)
	}
	var res ops.DraftHistoryListResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestListRoute_ReturnsEachListNewestFirstWithItsFields(t *testing.T) {
	f := rigs.StartDesigns(t)
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := drafthistory.NewWith(f.H.StateDir, 0, func() time.Time { tick = tick.Add(time.Minute); return tick })
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	save := func(l drafthistory.List, e drafthistory.Entry, imgs ...string) {
		t.Helper()
		if _, err := s.Save(l, e, imgs); err != nil {
			t.Fatal(err)
		}
	}
	save(drafthistory.ChatDrafts, drafthistory.Entry{Text: "old", Project: "proj", Name: "c1", Harness: "h", Model: "m", Effort: "high", Folder: "trunk"})
	save(drafthistory.ChatDrafts, drafthistory.Entry{Text: "", Project: "proj", Name: "c2"}, img)
	save(drafthistory.DesignDrafts, drafthistory.Entry{Text: "d", Project: "proj", Slug: "map"})
	save(drafthistory.Sent, drafthistory.Entry{Text: "s", Project: "proj", Composer: drafthistory.ComposerDesign, Slug: "map"})

	chat := listOK(t, f, "chat-drafts")
	if chat.Max != 50 || len(chat.Entries) != 2 {
		t.Fatalf("chat = %+v", chat)
	}
	if got := chat.Entries[0]; got.Text != "" || got.ImageCount != 1 || got.Name != "c2" || got.ImagesDropped {
		t.Errorf("images-only newest = %+v", got)
	}
	if got := chat.Entries[1]; got.Name != "c1" || got.Harness != "h" || got.Model != "m" || got.Effort != "high" || got.Folder != "trunk" || got.ImageCount != 0 {
		t.Errorf("oldest = %+v", got)
	}
	if d := listOK(t, f, "design-drafts"); len(d.Entries) != 1 || d.Entries[0].Slug != "map" || d.Entries[0].Project != "proj" {
		t.Errorf("design = %+v", d)
	}
	if sent := listOK(t, f, "sent"); len(sent.Entries) != 1 || sent.Entries[0].Composer != "design" || sent.Entries[0].Slug != "map" {
		t.Errorf("sent = %+v", sent)
	}
}

func TestListRoute_DroppedImagesReturnTheMarkNotACount(t *testing.T) {
	f := rigs.StartDesigns(t)
	s := drafthistory.NewWith(f.H.StateDir, 1, nil)
	img := filepath.Join(t.TempDir(), "big.png")
	if err := os.WriteFile(img, []byte("too big for a one byte cap"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(drafthistory.Sent, drafthistory.Entry{Text: "x"}, []string{img}); err != nil {
		t.Fatal(err)
	}
	e := listOK(t, f, "sent").Entries
	if len(e) != 1 || !e[0].ImagesDropped || e[0].ImageCount != 0 {
		t.Fatalf("entries = %+v", e)
	}
}

func TestListRoute_EmptyListIsAnEmptyArray(t *testing.T) {
	f := rigs.StartDesigns(t)
	code, out := listRoute(t, f, "design-drafts")
	if code != http.StatusOK || !strings.Contains(out, `"entries":[]`) {
		t.Fatalf("= %d %s", code, out)
	}
}

func TestListRoute_UnknownListIsRefused(t *testing.T) {
	f := rigs.StartDesigns(t)
	if code, out := listRoute(t, f, "nope"); code != http.StatusBadRequest {
		t.Fatalf("= %d %s", code, out)
	}
}
