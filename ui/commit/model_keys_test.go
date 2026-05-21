package commit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/nav"
)

func TestFilterLogRouteReturnsLogRouteForSelectedFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "foo.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.ready = true

	route := m.filterLogRoute()

	if route.Kind != nav.RouteLog {
		t.Fatalf("filterLogRoute kind = %q, want %q", route.Kind, nav.RouteLog)
	}
	if route.FilterPath != "foo.txt" {
		t.Fatalf("filterLogRoute FilterPath = %q, want 'foo.txt'", route.FilterPath)
	}
}

func TestFilterLogRouteIncludesLineRangeWhenDiffFocusedHunkMode(t *testing.T) {
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

	route := m.filterLogRoute()

	if route.FilterStartLine == 0 && route.FilterEndLine == 0 {
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
	if route, ok := nav.IsPush(msg); !ok {
		t.Fatalf("expected nav.Push message, got %T", msg)
	} else if route.Kind != nav.RouteLog {
		t.Fatalf("expected RouteLog, got %q", route.Kind)
	}
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
