package commit

import (
	"errors"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
)

func TestCommentLocationAndBodyWithValidDiffSelection(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "old-1\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "new-1\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeLine)

	data := m.diffModel.DataRef()
	data.ActiveLine = 0
	data.Parsed.Changed = []diffcore.ChangedLine{{NewLine: 1, Prefix: '-', Text: "old-1"}}
	data.RawLines = []string{"-old-1"}

	cl := m.commentLocationAndBody()

	if !cl.ok {
		t.Fatalf("expected ok=true from valid diff selection, errMsg=%q", cl.errMsg)
	}
}

func TestCommentLocationAndBodyFallsBackToRawLines(t *testing.T) {
	m := newTestModel(testutil.TempRepo(t), "HEAD")
	m.ready = true
	m.focusDiff = true

	// Set raw lines without a valid hunk/line selection.
	data := m.diffModel.DataRef()
	data.RawLines = []string{"some raw content"}
	data.ActiveHunk = -1
	data.ActiveLine = -1

	cl := m.commentLocationAndBody()

	if !cl.ok {
		t.Fatalf("expected ok=true when raw lines are available, errMsg=%q", cl.errMsg)
	}
	if len(cl.body) == 0 {
		t.Fatal("expected body from raw lines fallback")
	}
}

func TestCommentLocationAndBodyReturnsErrMsgWhenDiffModelEmpty(t *testing.T) {
	m := newTestModel(testutil.TempRepo(t), "HEAD")
	m.ready = true

	// Replace diffModel with a fully empty one (no raw lines, no hunks).
	m.diffModel = diffview.NewModel(false)

	cl := m.commentLocationAndBody()

	// Both RawLines and hunk selection are empty, so should fail.
	if cl.ok {
		t.Fatalf("expected ok=false when diffModel has no content, got ok=true body=%v", cl.body)
	}
	if cl.errMsg == "" {
		t.Fatal("expected non-empty errMsg when no selection")
	}
}

func TestHandleEditCommentFinishedWithError(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	m.ready = true

	updatedModel, cmd := m.handleEditCommentFinished(editCommentFinishedMsg{
		err: errors.New("editor failed"),
	})
	_ = updatedModel

	if cmd == nil {
		t.Fatal("expected error notify cmd from failed comment edit")
	}
}

func TestHandleEditCommentFinishedWithSplitApp(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	m.ready = true

	updatedModel, cmd := m.handleEditCommentFinished(editCommentFinishedMsg{
		splitApp: "kitty",
	})
	_ = updatedModel

	if cmd == nil {
		t.Fatal("expected notify cmd when splitApp is set")
	}
}

func TestHandleEditCommentFinishedSuccess(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	updatedModel, cmd := m.handleEditCommentFinished(editCommentFinishedMsg{})
	_ = updatedModel

	if cmd == nil {
		t.Fatal("expected notify cmd on successful comment edit")
	}
}
