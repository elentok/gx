package drafts

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire/ops"
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
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Project: "proj", Idea: "b", Revision: d.Revision})
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
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Project: "other", Idea: "a", Revision: d.Revision, KeepEditedAt: true})
	if err != nil || res.Draft == nil {
		t.Fatalf("save: %+v %v", res, err)
	}
	if !res.Draft.EditedAt.Equal(d.EditedAt) || res.Draft.Revision != d.Revision+1 || res.Draft.Project != "other" {
		t.Fatalf("want same edited-at, next revision, new project: %+v", res.Draft)
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
	res, err := s.Save(d.ID, ops.DesignDraftSaveRequest{Project: "proj", Revision: d.Revision})
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
