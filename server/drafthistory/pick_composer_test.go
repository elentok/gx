package drafthistory_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/agentfake"
)

func chatSession(t *testing.T) rigs.PromptFixture {
	t.Helper()
	return rigs.StartPromptSessionAs(t, 0, wire.CreateSessionRequest{Name: "my-chat"})
}

// composerImages saves one PNG per width into the session's image folder and
// returns the ids, like uploads from the composer.
func composerImages(t *testing.T, f rigs.PromptFixture, widths ...int) []string {
	t.Helper()
	var ids []string
	for _, w := range widths {
		res, err := imagestore.Save(sessionImages(f), pngOf(w))
		if err != nil || res.Refused {
			t.Fatalf("save image: %+v %v", res, err)
		}
		ids = append(ids, res.ID)
	}
	return ids
}

func sessionImages(f rigs.PromptFixture) string {
	return filepath.Join(f.H.SessionsDir, f.ID, "images")
}

func pickComposer(t *testing.T, f rigs.PromptFixture, entry, text string, images []string) ops.ComposerPickResult {
	t.Helper()
	body, _ := json.Marshal(ops.ComposerPickRequest{SessionID: f.ID, EntryID: entry, Text: text, Images: images})
	code, out := servertest.WebPostJSON(t, f.Base+"/v1/sessions/pick-history", string(body), f.Ck, "", servertest.JSONPost(f.Base))
	if code != http.StatusOK {
		t.Fatalf("pick = %d %s", code, out)
	}
	var res ops.ComposerPickResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func folderIDs(t *testing.T, f rigs.PromptFixture) []string {
	t.Helper()
	es, _ := os.ReadDir(sessionImages(f))
	var ids []string
	for _, e := range es {
		if len(e.Name()) > 0 && e.Name()[0] != '.' {
			ids = append(ids, e.Name())
		}
	}
	return ids
}

func TestPickComposer_ChatRestoresTextAndImagesAndKeepsTheSession(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	before := servertest.ReadMeta(t, f.H, f.ID)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "old words", Composer: drafthistory.ComposerChat, Name: "other", Harness: "codex"}, 3, 4)
	old := composerImages(t, f, 7)

	res := pickComposer(t, f, entry, "unsent", old)
	if res.Refused || res.Text != "old words" || len(res.Images) != 2 || len(res.Notices) != 0 {
		t.Fatalf("pick = %+v", res)
	}
	if got := len(folderIDs(t, f)); got != len(res.Images) {
		t.Errorf("image folder holds %d files, want only the %d new ones", got, len(res.Images))
	}
	for _, id := range append(slices.Clone(res.Images), old...) {
		_, found := imagestore.Find(sessionImages(f), id)
		if found != slices.Contains(res.Images, id) {
			t.Errorf("image %s found = %v in the session folder", id, found)
		}
	}
	after := servertest.ReadMeta(t, f.H, f.ID)
	for _, k := range []string{"harness", "model", "effort", "name"} {
		if before[k] != after[k] {
			t.Errorf("meta[%s] = %v after the pick, was %v", k, after[k], before[k])
		}
	}
}

func TestPickComposer_DesignRestores(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, 0)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "design words", Composer: drafthistory.ComposerDesign, Slug: "map"}, 3)
	res := pickComposer(t, f, entry, "", nil)
	if res.Refused || res.Text != "design words" || len(res.Images) != 1 {
		t.Fatalf("pick = %+v", res)
	}
}

func TestPickComposer_ACommandFromAnotherHarnessIsReturnedUnchanged(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "/compact now", Composer: drafthistory.ComposerChat, Harness: "claude"})
	res := pickComposer(t, f, entry, "", nil)
	if res.Text != "/compact now" {
		t.Errorf("text = %q, want it unchanged", res.Text)
	}
	if got := servertest.ReadMeta(t, f.H, f.ID)["harness"]; got != string(agentfake.HarnessKind) {
		t.Errorf("harness = %v, want unchanged", got)
	}
}

func TestPickComposer_SavesTheComposerBeforeReplacingIt(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "picked", Composer: drafthistory.ComposerChat})
	old := composerImages(t, f, 5)
	pickComposer(t, f, entry, "my draft", old)

	items := listOf(t, f.DesignsFixture, drafthistory.Sent)
	if len(items) != 2 || items[0].Text != "my draft" || items[0].ImageCount != 1 || items[0].Name != "my-chat" || items[0].Harness != string(agentfake.HarnessKind) {
		t.Fatalf("sent list = %+v, want the composer saved on top with its settings", items)
	}
	for _, id := range old {
		if _, ok := imagestore.Find(sessionImages(f), id); ok {
			t.Errorf("old image %s is still in the session folder", id)
		}
	}
}

func TestPickComposer_EmptyAndEqualComposersAddNothing(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "same", Composer: drafthistory.ComposerChat, Name: "my-chat"}, 3)
	first := pickComposer(t, f, entry, "  ", nil)
	if first.Text != "same" {
		t.Fatalf("pick into an empty composer = %+v", first)
	}
	if n := len(listOf(t, f.DesignsFixture, drafthistory.Sent)); n != 1 {
		t.Fatalf("sent has %d entries after an empty composer, want 1", n)
	}
	// The composer now equals the entry: picking it again changes nothing.
	res := pickComposer(t, f, entry, "same", first.Images)
	if res.Text != "same" || !slices.Equal(res.Images, first.Images) {
		t.Errorf("repeat pick = %+v, want the composer back as it was", res)
	}
	if n := len(listOf(t, f.DesignsFixture, drafthistory.Sent)); n != 1 {
		t.Errorf("sent has %d entries after an equal pick, want 1", n)
	}
}

func TestPickComposer_NeverMoreThanTenImages(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	widths := make([]int, 12)
	for i := range widths {
		widths[i] = i + 1
	}
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "many", Composer: drafthistory.ComposerChat}, widths...)
	res := pickComposer(t, f, entry, "", nil)
	if len(res.Images) != wire.MaxPromptImages || len(folderIDs(t, f)) != wire.MaxPromptImages {
		t.Errorf("pick gave %d images, folder holds %d, want %d", len(res.Images), len(folderIDs(t, f)), wire.MaxPromptImages)
	}
}

func TestPickComposer_EntryOutlivesItsSourceSession(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	ids := composerImages(t, f, 3)
	path, _ := imagestore.Find(sessionImages(f), ids[0])
	if _, err := drafthistory.New(f.H.StateDir).Save(drafthistory.Sent, drafthistory.Entry{Text: "kept", Composer: drafthistory.ComposerChat}, []string{path}); err != nil {
		t.Fatal(err)
	}
	entry := listOf(t, f.DesignsFixture, drafthistory.Sent)[0].ID
	// The source files are gone, as when the source session was removed.
	if err := os.RemoveAll(sessionImages(f)); err != nil {
		t.Fatal(err)
	}
	if res := pickComposer(t, f, entry, "", nil); res.Text != "kept" || len(res.Images) != 1 {
		t.Errorf("pick = %+v", res)
	}
}

func TestPickComposer_MissingImageFilesGiveANotice(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	entry := seedEntry(t, f.DesignsFixture, drafthistory.Sent, drafthistory.Entry{Text: "partly", Composer: drafthistory.ComposerChat}, 3, 4)
	picked, err := drafthistory.New(f.H.StateDir).Load(drafthistory.Sent, entry)
	paths := picked.Paths
	if err != nil || len(paths) != 2 {
		t.Fatalf("paths = %v %v", paths, err)
	}
	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	res := pickComposer(t, f, entry, "", nil)
	if res.Text != "partly" || len(res.Images) != 1 || !slices.Equal(res.Notices, []string{"1 images missing"}) {
		t.Errorf("pick = %+v", res)
	}
}

func TestPickComposer_ImagesNotKeptRestoresTextOnly(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	s := drafthistory.NewWith(f.H.StateDir, 10, nil)
	if _, err := s.Save(drafthistory.Sent, drafthistory.Entry{Text: "big", Composer: drafthistory.ComposerChat}, writePNGs(t, 20)); err != nil {
		t.Fatal(err)
	}
	items, _ := s.List(drafthistory.Sent)
	if !items[0].ImagesDropped {
		t.Fatal("entry should be marked images not kept")
	}
	res := pickComposer(t, f, items[0].ID, "", nil)
	if res.Text != "big" || len(res.Images) != 0 {
		t.Errorf("pick = %+v, want text only", res)
	}
}

func TestPickComposer_OtherSessionsAreRefusedAndNothingChanges(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "loop", "01", "broken", servertest.TicketOpts{PauseReason: "merge-conflict"})
	d := rigs.StartDesignsIn(t, store)
	fix := d.Fix(t, "proj:loop/01")
	if fix.Refused || fix.Session == nil {
		t.Fatalf("fix = %+v", fix)
	}
	entry := seedEntry(t, d, drafthistory.Sent, drafthistory.Entry{Text: "x", Composer: drafthistory.ComposerChat})
	f := rigs.PromptFixture{DesignsFixture: d, ID: fix.Session.ID}
	res := pickComposer(t, f, entry, "typed", nil)
	if !res.Refused || res.Reason != wire.ReasonSessionNotPlain {
		t.Fatalf("pick = %+v, want a refusal", res)
	}
	if n := len(listOf(t, d, drafthistory.Sent)); n != 1 {
		t.Errorf("sent has %d entries, want 1", n)
	}
}

func TestPickComposer_AGoneEntryIsRefused(t *testing.T) {
	t.Parallel()
	f := chatSession(t)
	res := pickComposer(t, f, "nope", "typed", nil)
	if !res.Refused || res.Reason != ops.ReasonHistoryEntryGone {
		t.Errorf("pick = %+v", res)
	}
}
