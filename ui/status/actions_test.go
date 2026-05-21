package status

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/status/diffarea"
)

func TestPushKeyOpensPushModel(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "init")
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/test")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P", ShiftedCode: 'P'})
	m = updated.(Model)

	if !m.push.IsOpen {
		t.Fatalf("expected push key to open push.Model")
	}
}

func TestRebaseKeyOpensSpecificConfirm(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "init")
	testutil.MustGitExported(t, repo, "checkout", "-b", "feature/test")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)

	if !m.confirmOpen {
		t.Fatalf("expected rebase key to open confirmation")
	}
	if !strings.Contains(m.confirmTitle, "Rebase branch feature/test on origin/main?") {
		t.Fatalf("unexpected rebase confirm title: %q", m.confirmTitle)
	}
}

func TestFilterLogRouteReturnsFilePathWhenFileSelected(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "foo.txt", "hello\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
	m.ready = true

	route := m.filterLogRoute()

	if route.Kind != nav.RouteLog {
		t.Fatalf("filterLogRoute kind = %q, want %q", route.Kind, nav.RouteLog)
	}
	if route.FilterPath != "foo.txt" {
		t.Fatalf("filterLogRoute FilterPath = %q, want 'foo.txt'", route.FilterPath)
	}
}

func TestFilterLogRouteIncludesLineRangeInDiffHunkMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "bar.txt", "old-1\nold-2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "bar.txt", "new-1\nnew-2\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)

	// Inject hunk data directly so we can test the line-range path.
	data := m.diffarea.Unstaged.DataRef()
	data.ActiveHunk = 0
	data.Parsed.Hunks = []diffcore.ParsedHunk{{NewStart: 5, NewCount: 3}}

	route := m.filterLogRoute()

	if route.Kind != nav.RouteLog {
		t.Fatalf("filterLogRoute kind = %q, want %q", route.Kind, nav.RouteLog)
	}
	if route.FilterPath != "bar.txt" {
		t.Fatalf("filterLogRoute FilterPath = %q, want 'bar.txt'", route.FilterPath)
	}
	if route.FilterStartLine == 0 && route.FilterEndLine == 0 {
		t.Fatal("expected non-zero line range from diff hunk mode")
	}
}

func TestActiveLogLineRangeInHunkMode(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.diffarea.SetNavMode(diffview.NavModeHunk)

	data := m.diffarea.Unstaged.DataRef()
	data.ActiveHunk = 0
	data.Parsed.Hunks = []diffcore.ParsedHunk{{NewStart: 10, NewCount: 5}}

	start, end := m.activeLogLineRange()
	if start != 10 || end != 14 {
		t.Fatalf("activeLogLineRange in hunk mode = (%d, %d), want (10, 14)", start, end)
	}
}

func TestActiveLogLineRangeInLineModeWithNewLine(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.diffarea.SetNavMode(diffview.NavModeLine)

	data := m.diffarea.Unstaged.DataRef()
	data.ActiveLine = 0
	data.Parsed.Changed = []diffcore.ChangedLine{{NewLine: 7}}

	start, end := m.activeLogLineRange()
	if start != 7 || end != 7 {
		t.Fatalf("activeLogLineRange in line mode = (%d, %d), want (7, 7)", start, end)
	}
}

func TestActiveLogLineRangeReturnsZeroWhenNoActiveHunk(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.diffarea.Unstaged.DataRef().ActiveHunk = -1

	start, end := m.activeLogLineRange()
	if start != 0 || end != 0 {
		t.Fatalf("expected (0,0) with no active hunk, got (%d,%d)", start, end)
	}
}

func TestFilterLogRouteGHKeyPushesNavRoute(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "nav.txt", "content\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
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

func TestAppendRunningOutputStripsTerminalControlSequences(t *testing.T) {
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.openRunning("Amend", newStageActionRunner(actionAmend, m.worktreeRoot, "", ""))

	m.appendRunningOutput("\x1b[32mWriting objects: 100%\x1b[0m\r\n")
	m.appendRunningOutput("remote: \x1b]8;;https://example.com\x07https://example.com\x1b]8;;\x07\n")

	if strings.Contains(m.runningContent, "\x1b") {
		t.Fatalf("expected running content to strip ANSI escapes, got %q", m.runningContent)
	}
	if strings.Contains(m.runningContent, "\r") {
		t.Fatalf("expected running content to strip carriage returns, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "Writing objects: 100%") {
		t.Fatalf("expected sanitized progress text, got %q", m.runningContent)
	}
	if !strings.Contains(m.runningContent, "remote: https://example.com") {
		t.Fatalf("expected sanitized hyperlink text, got %q", m.runningContent)
	}
}
