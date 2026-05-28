package commit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/nav"
)

func TestFilterLogViewStateReturnsLogViewStateForSelectedFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "foo.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	vs := m.filterLogViewState()

	if vs.Tab != nav.TabLog {
		t.Fatalf("filterLogViewState tab = %q, want %q", vs.Tab, nav.TabLog)
	}
	if vs.FilterPath != "foo.txt" {
		t.Fatalf("filterLogViewState FilterPath = %q, want 'foo.txt'", vs.FilterPath)
	}
}

func TestFilterLogViewStateIncludesLineRangeWhenDiffFocusedHunkMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "bar.txt", "old-1\nold-2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "bar.txt", "new-1\nnew-2\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeHunk)

	data := m.diffModel.DataRef()
	data.ActiveHunk = 0
	data.Parsed.Hunks = []diffcore.ParsedHunk{{NewStart: 3, NewCount: 2}}

	vs := m.filterLogViewState()

	if vs.FilterStartLine == 0 && vs.FilterEndLine == 0 {
		t.Fatal("expected non-zero line range when diff focused in hunk mode")
	}
}

func TestActiveLogLineRangeInHunkModeReturnsHunkBounds(t *testing.T) {
	m := newTestModel(testutil.TempRepo(t), "HEAD")
	m.ready = true
	m.diffModel.SetNavMode(diffview.NavModeHunk)

	data := m.diffModel.DataRef()
	data.ActiveHunk = 0
	data.Parsed.Hunks = []diffcore.ParsedHunk{{NewStart: 10, NewCount: 5}}

	start, end := m.activeLogLineRange()
	if start != 10 || end != 14 {
		t.Fatalf("activeLogLineRange in hunk mode = (%d, %d), want (10, 14)", start, end)
	}
}

func TestActiveLogLineRangeInLineModeReturnsChangedLine(t *testing.T) {
	m := newTestModel(testutil.TempRepo(t), "HEAD")
	m.ready = true
	m.diffModel.SetNavMode(diffview.NavModeLine)

	data := m.diffModel.DataRef()
	data.ActiveLine = 0
	data.Parsed.Changed = []diffcore.ChangedLine{{NewLine: 7}}

	start, end := m.activeLogLineRange()
	if start != 7 || end != 7 {
		t.Fatalf("activeLogLineRange in line mode = (%d, %d), want (7, 7)", start, end)
	}
}

func TestActiveLogLineRangeReturnsZeroWhenNoActiveHunk(t *testing.T) {
	m := newTestModel(testutil.TempRepo(t), "HEAD")
	m.ready = true
	m.diffModel.SetNavMode(diffview.NavModeHunk)
	m.diffModel.DataRef().ActiveHunk = -1

	start, end := m.activeLogLineRange()
	if start != 0 || end != 0 {
		t.Fatalf("expected (0,0) with no active hunk, got (%d,%d)", start, end)
	}
}

func TestFilterLogGHKeyPushesNavRoute(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "nav.txt", "content\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("g+h should produce a nav.Push command")
	}
	msg := cmd()
	if vs, ok := nav.IsOpen(msg); !ok {
		t.Fatalf("expected nav.Push message, got %T", msg)
	} else if vs.Tab != nav.TabLog {
		t.Fatalf("expected TabLog, got %q", vs.Tab)
	}
}

func TestGChordBindingsReachableFromDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "nav.txt", "content\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "nav.txt", "content changed\n")
	testutil.CommitAll(t, repo, "change")

	newDiffFocusModel := func(t *testing.T) Model {
		t.Helper()
		m := newTestModel(repo, "HEAD")
		m.ready = true
		m.width = 100
		m.height = 24
		m.syncDiffViewport()
		m.focusDiff = true
		return m
	}

	t.Run("g+esc cancels chord without leaving diff focus", func(t *testing.T) {
		m := newDiffFocusModel(t)
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
		m = updated.(Model)
		if !m.diffModel.HasPendingChord() {
			t.Fatal("after first g: expected diffview pending chord=true")
		}
		if len(m.keys.Prefix()) != 1 || m.keys.Prefix()[0] != "g" {
			t.Fatalf("after first g: expected commit prefix [g], got %v", m.keys.Prefix())
		}

		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m = updated.(Model)
		if m.diffModel.HasPendingChord() {
			t.Fatal("after g+esc: expected diffview pending chord=false")
		}
		if len(m.keys.Prefix()) != 0 {
			t.Fatalf("after g+esc: expected prefix cleared, got %v", m.keys.Prefix())
		}
		if !m.focusDiff {
			t.Fatal("after g+esc: expected to remain in diff focus")
		}
	})

	t.Run("g+g still routes to diffview", func(t *testing.T) {
		m := newDiffFocusModel(t)
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
		m = updated.(Model)
		if !m.diffModel.HasPendingChord() {
			t.Fatal("after first g: expected diffview pending chord=true")
		}

		updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
		m = updated.(Model)
		if m.diffModel.HasPendingChord() {
			t.Fatal("after g+g: expected diffview pending chord=false")
		}
		if len(m.keys.Prefix()) != 0 {
			t.Fatalf("after g+g: expected commit prefix cleared, got %v", m.keys.Prefix())
		}
	})

	t.Run("g+h remains available from diff focus", func(t *testing.T) {
		m := newDiffFocusModel(t)
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
		m = updated.(Model)
		updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
		m = updated.(Model)

		if cmd == nil {
			t.Fatal("g+h should produce a nav.Push command from diff focus")
		}
		msg := cmd()
		if vs, ok := nav.IsOpen(msg); !ok {
			t.Fatalf("expected nav.Push message, got %T", msg)
		} else if vs.Tab != nav.TabLog {
			t.Fatalf("expected TabLog, got %q", vs.Tab)
		}
		if m.diffModel.HasPendingChord() {
			t.Fatal("after g+h: expected diffview pending chord=false")
		}
		if len(m.keys.Prefix()) != 0 {
			t.Fatalf("after g+h: expected prefix cleared, got %v", m.keys.Prefix())
		}
	})
}

func TestDispatchBindingAmendOpensConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	// Stage a new change so amend.Open succeeds (it requires staged files).
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A", ShiftedCode: 'A'})
	m = updated.(Model)

	if !m.amendConfirm.IsOpen {
		t.Fatal("A key should open amend confirm dialog")
	}
}

func TestDispatchBindingRefreshMenuSendsNotify(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)

	if cmd == nil {
		t.Fatal("m+r should return a notify command")
	}
}
