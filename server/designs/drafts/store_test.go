package drafts

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	return s
}

func mustCreate(t *testing.T, s *Store, idea string) *ops.DesignDraft {
	t.Helper()
	d, err := s.Create("proj", "", idea)
	if err != nil || d == nil {
		t.Fatalf("create: %v %v", d, err)
	}
	return d
}

func TestSave_CurrentRevisionBumpsAndStamps(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "b", Revision: d.Revision})
	if err != nil || res.Refused || res.Draft == nil {
		t.Fatalf("save: %+v %v", res, err)
	}
	if res.Draft.Revision != d.Revision+1 || !res.Draft.EditedAt.After(d.EditedAt) || res.Draft.Idea != "b" {
		t.Fatalf("draft not bumped: %+v", res.Draft)
	}
}

func TestSave_KeepEditedAtLeavesTimeAndBumpsRevision(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "a", Revision: d.Revision, KeepEditedAt: true})
	if err != nil || res.Draft == nil {
		t.Fatalf("save: %+v %v", res, err)
	}
	if !res.Draft.EditedAt.Equal(d.EditedAt) || res.Draft.Revision != d.Revision+1 {
		t.Fatalf("want same edited-at and next revision: %+v", res.Draft)
	}
}

func TestSetTarget_KeepsRevisionMovesEditedAt(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	res, err := s.SetTarget(d.ID, "other", "a-slug", false)
	if err != nil || res.Refused || res.Draft == nil {
		t.Fatalf("set target: %+v %v", res, err)
	}
	got := res.Draft
	if got.Revision != d.Revision || !got.EditedAt.After(d.EditedAt) || got.Slug != "a-slug" || got.Project != "other" || got.Label != "a-slug" {
		t.Fatalf("draft = %+v", got)
	}
	res, _ = s.SetTarget(d.ID, "other", "", true)
	if res.Draft.Slug != "" || !res.Draft.EditedAt.Equal(got.EditedAt) {
		t.Fatalf("empty slug / keepEditedAt: %+v", res.Draft)
	}
}

func TestSetTarget_MissingDraftIsGone(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"0123456789abcdef", "../x"} {
		res, err := s.SetTarget(id, "p", "s", false)
		if err != nil || !res.Refused || res.Reason != ops.ReasonDraftGone {
			t.Errorf("%s: %+v %v, want draft-gone", id, res, err)
		}
	}
}

func TestSave_AfterTargetKeepsSlugAndProject(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	if _, err := s.SetTarget(d.ID, "other", "new-slug", false); err != nil {
		t.Fatal(err)
	}
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "b", Revision: d.Revision})
	if err != nil || res.Draft == nil || res.Draft.Slug != "new-slug" || res.Draft.Project != "other" {
		t.Fatalf("save after target: %+v %v", res, err)
	}
	stale, _ := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "c", Revision: d.Revision})
	if !stale.Refused || stale.Reason != ops.ReasonDraftRevisionConflict {
		t.Fatalf("want conflict, got %+v", stale)
	}
}

func writeOld(t *testing.T, s *Store, id, slug, name, idea string) {
	t.Helper()
	rec := map[string]any{"id": id, "project": "p", "slug": slug, "idea": idea, "images": []string{}, "revision": 1, "edited_at": "2026-01-01T00:00:00Z"}
	if name != "-none-" {
		rec["name"] = name
	}
	b, _ := json.Marshal(rec)
	if err := os.MkdirAll(s.dir(id), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.record(id), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateNames(t *testing.T) {
	cases := []struct {
		name, slug, oldName, idea string
		wantSlug, wantIdea        string
	}{
		{"valid name becomes slug", "", "My Cool Idea", "", "my-cool-idea", ""},
		{"invalid name leads the prompt", "", "!!!", "body", "", "!!!\nbody"},
		{"name-only invalid has no trailing newline", "", "!!!", "", "", "!!!"},
		{"existing slug kept, name joins prompt", "keep", "Other Name", "body", "keep", "Other Name\nbody"},
		{"no name untouched", "s", "-none-", "body", "s", "body"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			id := fmt.Sprintf("%016x", i+1)
			writeOld(t, s, id, tc.slug, tc.oldName, tc.idea)
			if err := s.MigrateNames(); err != nil {
				t.Fatal(err)
			}
			d, err := s.Get(id)
			if err != nil || d.Slug != tc.wantSlug || d.Idea != tc.wantIdea {
				t.Fatalf("draft = %+v, %v; want slug %q idea %q", d, err, tc.wantSlug, tc.wantIdea)
			}
			b1, _ := os.ReadFile(s.record(id))
			if strings.Contains(string(b1), `"name"`) {
				t.Errorf("name key survived: %s", b1)
			}
			if err := s.MigrateNames(); err != nil {
				t.Fatal(err)
			}
			if b2, _ := os.ReadFile(s.record(id)); string(b1) != string(b2) {
				t.Errorf("second run changed the record:\n%s\n%s", b1, b2)
			}
		})
	}
}

func TestSave_OldRevisionConflictLeavesFile(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	if _, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "b", Revision: d.Revision}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.record(d.ID))
	res, _ := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "stale", Revision: d.Revision})
	if !res.Refused || res.Reason != ops.ReasonDraftRevisionConflict {
		t.Fatalf("want conflict, got %+v", res)
	}
	after, _ := os.ReadFile(s.record(d.ID))
	if string(before) != string(after) {
		t.Fatal("conflicting save changed the file")
	}
}

func TestSave_DeletedDraftIsGone(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	if err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "x", Revision: d.Revision})
	if err != nil || !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Fatalf("want gone, got %+v %v", res, err)
	}
}

func TestSave_EmptyingKeepsTheDraft(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Revision: d.Revision})
	if err != nil || res.Deleted || res.Draft == nil || res.Draft.Revision != d.Revision+1 {
		t.Fatalf("want kept draft, got %+v %v", res, err)
	}
	if res.Draft.Label != "Untitled design" {
		t.Errorf("label = %q", res.Draft.Label)
	}
	if _, err := s.Get(d.ID); err != nil {
		t.Fatalf("get after emptying: %v", err)
	}
}

func TestIsEmpty(t *testing.T) {
	cases := []struct {
		name string
		d    ops.DesignDraft
		want bool
	}{
		{"nothing", ops.DesignDraft{}, true},
		{"project only", ops.DesignDraft{Project: "p"}, true},
		{"slug", ops.DesignDraft{Slug: "s"}, false},
		{"prompt", ops.DesignDraft{Idea: "x"}, false},
		{"image", ops.DesignDraft{Images: []string{"i"}}, false},
	}
	for _, c := range cases {
		if got := isEmpty(&c.d); got != c.want {
			t.Errorf("%s: isEmpty = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLabel(t *testing.T) {
	cases := []struct{ name, slug, prompt, want string }{
		{"slug wins", "my-slug", "some prompt", "my-slug"},
		{"first five words", "", "one two three four five six seven", "one two three four five"},
		{"whitespace collapsed", "", "  a\n b   c ", "a b c"},
		{"cut at 40", "", "aaaaaaaaaaaa bbbbbbbbbbbb cccccccccccc dddddddddddd", "aaaaaaaaaaaa bbbbbbbbbbbb cccccccccccc d"},
		{"fallback", "", "  \n ", "Untitled design"},
	}
	for _, c := range cases {
		if got := Label(c.slug, c.prompt); got != c.want {
			t.Errorf("%s: Label = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSave_KeepsImageList(t *testing.T) {
	s := newStore(t)
	d := mustCreate(t, s, "a")
	d.Images = []string{"img1"}
	if err := s.write(d); err != nil {
		t.Fatal(err)
	}
	res, _ := s.Save(d.ID, ops.DesignDraftSaveRequest{Revision: d.Revision})
	if res.Deleted || res.Draft == nil || len(res.Draft.Images) != 1 {
		t.Fatalf("a draft with an image is not empty: %+v", res)
	}
}

func TestCreate_EmptyIsKept(t *testing.T) {
	s := newStore(t)
	d, err := s.Create("proj", "", "")
	if err != nil || d == nil {
		t.Fatalf("got %v %v", d, err)
	}
	items, _ := s.List()
	if len(items) != 1 || items[0].Label != "Untitled design" {
		t.Fatalf("list = %+v", items)
	}
}

func TestListOrderedByEditedAtAndReadChangesNothing(t *testing.T) {
	s := newStore(t)
	a := mustCreate(t, s, "a")
	b := mustCreate(t, s, "b")
	if _, err := s.Save(a.ID, ops.DesignDraftSaveRequest{Idea: "a2", Revision: a.Revision}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.List()
	if len(items) != 2 || items[0].ID != a.ID || items[1].ID != b.ID {
		t.Fatalf("order: %+v", items)
	}
	got, _ := s.Get(b.ID)
	if !got.EditedAt.Equal(b.EditedAt) || got.Revision != b.Revision {
		t.Fatalf("read changed the draft: %+v", got)
	}
	items, _ = s.List()
	if items[0].ID != a.ID {
		t.Fatal("reading reordered the list")
	}
}

func TestSave_ConcurrentSameRevisionOneWinner(t *testing.T) {
	s := New(t.TempDir())
	d := mustCreate(t, s, "a")
	const n = 8
	results := make(chan ops.DesignDraftSaveResult, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "x", Revision: d.Revision})
			if err != nil {
				t.Error(err)
			}
			results <- res
		})
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for r := range results {
		switch {
		case r.Draft != nil:
			wins++
		case r.Reason == ops.ReasonDraftRevisionConflict:
			conflicts++
		}
	}
	if wins != 1 || conflicts != n-1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestLastProject(t *testing.T) {
	s := newStore(t)
	if p, err := s.LastProject(); p != "" || err != nil {
		t.Fatalf("fresh: %q %v", p, err)
	}
	if err := s.SetLastProject("one"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastProject("two"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.LastProject(); p != "two" {
		t.Fatalf("got %q", p)
	}
	d := mustCreate(t, s, "a")
	if err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.LastProject(); p != "two" {
		t.Fatalf("delete cleared it: %q", p)
	}
	items, _ := s.List()
	if len(items) != 0 {
		t.Fatalf("last-used file listed as a draft: %v", items)
	}
}

func mustDeleteIfEmpty(t *testing.T, s *Store, id string) ops.DesignDraftSaveResult {
	t.Helper()
	res, err := s.DeleteIfEmpty(id)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDeleteIfEmpty(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	empty := mustCreate(t, s, "")
	if err := os.MkdirAll(s.ImagesDir(empty.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if res := mustDeleteIfEmpty(t, s, empty.ID); !res.Deleted {
		t.Fatalf("empty draft: %+v, want deleted", res)
	}
	if _, err := os.Stat(s.dir(empty.ID)); !os.IsNotExist(err) {
		t.Errorf("draft folder still there: %v", err)
	}
	if res := mustDeleteIfEmpty(t, s, empty.ID); res.Deleted || res.Reason != ops.ReasonDraftGone {
		t.Errorf("second delete: %+v, want draft-gone", res)
	}

	withIdea := mustCreate(t, s, "words")
	withImage := mustCreate(t, s, "")
	withImage.Images = []string{"img1"}
	if err := s.write(withImage); err != nil {
		t.Fatal(err)
	}
	withSlug, err := s.Create("proj", "a-slug", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []*ops.DesignDraft{withIdea, withImage, withSlug} {
		if res := mustDeleteIfEmpty(t, s, d.ID); res.Deleted || res.Reason != ops.ReasonDraftNotEmpty {
			t.Errorf("draft %+v: %+v, want kept as draft-not-empty", d, res)
		}
	}

	if _, err := s.SetTarget(withSlug.ID, "proj", "", false); err != nil {
		t.Fatal(err)
	}
	if res := mustDeleteIfEmpty(t, s, withSlug.ID); !res.Deleted {
		t.Errorf("after clearing the slug: %+v, want deleted", res)
	}
}

func TestDeleteIfEmpty_TextSavedAfterTheCallersReadKeepsTheDraft(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	d := mustCreate(t, s, "")
	// The caller saw an empty draft; another device then saved text.
	if _, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Idea: "typed elsewhere", Revision: d.Revision}); err != nil {
		t.Fatal(err)
	}
	if res := mustDeleteIfEmpty(t, s, d.ID); res.Deleted {
		t.Fatalf("got %+v, want the draft kept", res)
	}
	if _, err := s.Get(d.ID); err != nil {
		t.Errorf("draft lost: %v", err)
	}
}

// fakeClock makes the store's time move only when the test says so.
func fakeClock(s *Store) *time.Time {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return &now
}

func TestLease(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := fakeClock(s)
	d := mustCreate(t, s, "")

	if s.leased(d.ID) {
		t.Fatal("leased before any ping")
	}
	if res, err := s.Lease(d.ID); err != nil || res.Refused {
		t.Fatalf("ping = %+v, %v", res, err)
	}
	if !s.leased(d.ID) {
		t.Fatal("not leased after a ping")
	}
	*now = now.Add(LeaseTTL - time.Second)
	if !s.leased(d.ID) {
		t.Error("lease ended before the TTL")
	}
	if _, err := s.Lease(d.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(LeaseTTL - time.Second)
	if !s.leased(d.ID) {
		t.Error("a second ping did not extend the lease")
	}
	*now = now.Add(2 * time.Second)
	if s.leased(d.ID) {
		t.Error("lease still live after the TTL")
	}

	if res, err := s.Lease("0123456789abcdef"); err != nil || !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("ping on a missing draft = %+v, %v; want draft-gone", res, err)
	}
}

func TestSweepEmpty(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := fakeClock(s)
	empty := mustCreate(t, s, "")
	leased := mustCreate(t, s, "")
	skipped := mustCreate(t, s, "")
	kept := mustCreate(t, s, "words")
	if _, err := s.Lease(leased.ID); err != nil {
		t.Fatal(err)
	}

	removed, err := s.SweepEmpty(skipped.ID)
	if err != nil || len(removed) != 1 || removed[0] != empty.ID {
		t.Fatalf("sweep = %v, %v; want only %s", removed, err, empty.ID)
	}
	if _, err := s.Get(empty.ID); err != ErrNotFound {
		t.Errorf("swept draft reads %v, want ErrNotFound", err)
	}
	for _, d := range []*ops.DesignDraft{leased, skipped, kept} {
		if _, err := s.Get(d.ID); err != nil {
			t.Errorf("draft %s lost: %v", d.ID, err)
		}
	}

	*now = now.Add(LeaseTTL + time.Second)
	removed, err = s.SweepEmpty()
	if err != nil || len(removed) != 2 {
		t.Fatalf("sweep after expiry = %v, %v; want the leased and skipped drafts", removed, err)
	}
	if _, err := s.Get(kept.ID); err != nil {
		t.Errorf("non-empty draft lost: %v", err)
	}
}

func TestSweepEmpty_NoRootIsFine(t *testing.T) {
	t.Parallel()
	if removed, err := New(t.TempDir()).SweepEmpty(); err != nil || len(removed) != 0 {
		t.Fatalf("sweep = %v, %v", removed, err)
	}
}

func TestMemberKey(t *testing.T) {
	t.Parallel()
	if _, _, ok := tickets.ParseEpicAddress(MemberKey("0123456789abcdef")); ok {
		t.Errorf("MemberKey parses as an epic address")
	}
}

func TestStartRecord(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	now := fakeClock(s)
	const id = "0123456789abcdef"
	if got := s.Started(id); got != "" {
		t.Fatalf("Started before any record = %q", got)
	}
	if err := s.RecordStart(id, "proj:map"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(startRecordTTL - time.Hour)
	if got := s.Started(id); got != "proj:map" {
		t.Errorf("Started at 6d23h = %q, want proj:map", got)
	}
	*now = now.Add(time.Hour + time.Second)
	if got := s.Started(id); got != "" {
		t.Errorf("Started after 7 days = %q, want pruned", got)
	}
	if _, err := os.Stat(s.startPath(id)); !os.IsNotExist(err) {
		t.Errorf("record file still there: %v", err)
	}
}
