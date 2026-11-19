package chatdrafts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

func (r *startRig) history(t *testing.T) []drafthistory.Item {
	t.Helper()
	items, err := r.c.history.List(drafthistory.ChatDrafts)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// breakHistory puts a file where the history root belongs, so every save fails.
func (r *startRig) breakHistory(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.stateDir, "draft-history"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStart_SavesOneHistoryEntryAndAFailedLaunchRetryAddsNone(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	r.createRes = func() (wire.SessionResult, error) { return wire.SessionResult{}, errors.New("boom") }
	d := r.draft(t, ops.ChatDraft{Name: "n", Harness: "claude", Model: "m", Effort: "high", Brief: "the brief"})
	for range 2 {
		if res, _ := r.c.Start(d.ID); !res.Refused {
			t.Fatalf("start = %+v", res)
		}
	}
	got := r.history(t)
	if len(got) != 1 {
		t.Fatalf("entries after failed launches = %d, want 1", len(got))
	}
	e := got[0].Entry
	if e.Text != "the brief" || e.Project != "proj" || e.Name != "n" || e.Harness != "claude" ||
		e.Model != "m" || e.Effort != "high" || e.Folder != ops.ChatDraftFolderTrunk {
		t.Errorf("entry = %+v", e)
	}
	r.createRes = func() (wire.SessionResult, error) {
		return wire.SessionResult{Session: &wire.SessionInfo{ID: "sess-1"}}, nil
	}
	if res, _ := r.c.Start(d.ID); res.Started != "sess-1" {
		t.Fatalf("retry = %+v", res)
	}
	if n := len(r.history(t)); n != 1 {
		t.Errorf("entries after successful Start = %d, want 1", n)
	}
}

func TestStart_EmptyBriefSavesNoHistoryEntry(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	d.Brief = ""
	if err := r.c.store.write(d); err != nil {
		t.Fatal(err)
	}
	if res, _ := r.c.Start(d.ID); !res.Refused {
		t.Fatalf("start = %+v", res)
	}
	if n := len(r.history(t)); n != 0 {
		t.Errorf("entries = %d, want 0", n)
	}
}

func TestStart_FailedHistorySaveStillStarts(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	r.breakHistory(t)
	if res, err := r.c.Start(d.ID); err != nil || res.Started != "sess-1" {
		t.Fatalf("start = %+v, %v", res, err)
	}
}

func TestDeleteDraft_SavesBeforeRemovingAndAGoneIdAddsNone(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{Brief: "keep me"})
	res, err := r.c.deleteDraft(d.ID)
	if err != nil || !res.Deleted {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	if r.c.store.Exists(d.ID) {
		t.Error("draft survived the delete")
	}
	if got := r.history(t); len(got) != 1 || got[0].Text != "keep me" {
		t.Errorf("history = %+v", got)
	}
	if res, _ := r.c.deleteDraft(d.ID); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("second delete = %+v", res)
	}
	if n := len(r.history(t)); n != 1 {
		t.Errorf("entries after gone delete = %d, want 1", n)
	}
}

func TestStart_HistoryEntryHoldsACopyOfTheImages(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	if res, err := r.c.store.AddImage(d.ID, testPNG); err != nil || res.Refused {
		t.Fatalf("add image = %+v, %v", res, err)
	}
	if res, _ := r.c.Start(d.ID); res.Started == "" {
		t.Fatalf("start = %+v", res)
	}
	got := r.history(t)
	if len(got) != 1 || got[0].ImageCount != 1 {
		t.Fatalf("history = %+v", got)
	}
	paths, err := r.c.history.ImagePaths(drafthistory.ChatDrafts, got[0].ID)
	if err != nil || len(paths) != 1 {
		t.Fatalf("image paths = %v, %v", paths, err)
	}
	if b, err := os.ReadFile(paths[0]); err != nil || string(b) != string(testPNG) {
		t.Errorf("copy = %v bytes, %v", len(b), err)
	}
}

func TestDeleteDraft_FailedSaveKeepsTheDraft(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	r.breakHistory(t)
	if _, err := r.c.deleteDraft(d.ID); err == nil {
		t.Fatal("delete succeeded with a failing save")
	}
	if !r.c.store.Exists(d.ID) {
		t.Error("draft was removed although its save failed")
	}
}
