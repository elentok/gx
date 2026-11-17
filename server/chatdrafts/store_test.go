package chatdrafts

import (
	"testing"

	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/wire/ops"
)

func newDraft(t *testing.T, s *Store, brief string) *ops.ChatDraft {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	d := &ops.ChatDraft{ID: id, Project: "proj", Brief: brief, Folder: ops.ChatDraftFolderTrunk}
	if err := s.Create(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStore_CreateGetList(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "one two three four five six seven")
	if d.Revision != 1 || d.EditedAt.IsZero() || d.Label != "one two three four five" {
		t.Fatalf("created = %+v", d)
	}
	got, err := s.Get(d.ID)
	if err != nil || got.Brief != d.Brief || got.Label != d.Label || !s.Exists(d.ID) {
		t.Fatalf("get = %+v, %v", got, err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].ID != d.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestStore_LabelPrefersNameThenFallsBack(t *testing.T) {
	t.Parallel()
	if got := Label("named", "brief"); got != "named" {
		t.Errorf("name label = %q", got)
	}
	if got := Label("", "  "); got != "Untitled chat" {
		t.Errorf("empty label = %q", got)
	}
}

func TestStore_SaveMergesOnlyTheSettingsGivenAndKeepsStartState(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "first")
	d.Name, d.Model, d.WorktreePath, d.StartError = "old", "opus", "/wt/made", "boom"
	if err := s.write(d); err != nil {
		t.Fatal(err)
	}
	name, harness := " new ", "codex"
	res, err := s.Save(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "second", Revision: 1, Name: &name, Harness: &harness})
	if err != nil || res.Draft == nil {
		t.Fatalf("save = %+v, %v", res, err)
	}
	got := res.Draft
	if got.Name != "new" || got.Harness != "codex" || got.Model != "opus" || got.Revision != 2 ||
		got.WorktreePath != "/wt/made" || got.StartError != "boom" {
		t.Errorf("saved = %+v", got)
	}
}

func TestStore_SaveStaleRevisionIsAConflict(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "first")
	res, err := s.Save(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "second", Revision: 1})
	if err != nil || res.Draft == nil || res.Draft.Revision != 2 || res.Draft.Brief != "second" {
		t.Fatalf("save = %+v, %v", res, err)
	}
	res, err = s.Save(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "third", Revision: 1})
	if err != nil || !res.Refused || res.Reason != ops.ReasonDraftRevisionConflict {
		t.Fatalf("stale save = %+v, %v", res, err)
	}
	if got, _ := s.Get(d.ID); got.Brief != "second" {
		t.Errorf("brief after stale save = %q", got.Brief)
	}
}

func TestStore_SaveChangesOnlyTheBrief(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "first")
	if _, err := s.Save(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "second", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(d.ID)
	want := *d
	want.Brief, want.Revision, want.EditedAt, want.Label = "second", 2, got.EditedAt, got.Label
	if *got != want {
		t.Errorf("after save = %+v, want %+v", *got, want)
	}
}

func TestStore_DeleteAndGoneIDs(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	d := newDraft(t, s, "x")
	if existed, err := s.Delete(d.ID); err != nil || !existed {
		t.Fatalf("delete = %v, %v", existed, err)
	}
	if existed, err := s.Delete(d.ID); err != nil || existed {
		t.Errorf("second delete = %v, %v", existed, err)
	}
	if res, _ := s.Save(ops.ChatDraftSaveRequest{ID: d.ID, Revision: 1}); res.Reason != ops.ReasonDraftGone {
		t.Errorf("save of deleted = %+v", res)
	}
	for _, id := range []string{d.ID, "", "../x", "abc", "ABCDEF0123456789"} {
		if _, err := s.Get(id); err != ErrNotFound {
			t.Errorf("Get(%q) = %v, want ErrNotFound", id, err)
		}
	}
}

func TestMemberKey_IsNotADesignDraftKey(t *testing.T) {
	t.Parallel()
	key := MemberKey("0123456789abcdef")
	if id, ok := ParseMemberKey(key); !ok || id != "0123456789abcdef" {
		t.Errorf("ParseMemberKey(%q) = %q, %v", key, id, ok)
	}
	if _, ok := drafts.ParseMemberKey(key); ok {
		t.Errorf("drafts.ParseMemberKey parsed %q", key)
	}
	if _, ok := ParseMemberKey(drafts.MemberKey("0123456789abcdef")); ok {
		t.Error("ParseMemberKey parsed a Design draft key")
	}
}
