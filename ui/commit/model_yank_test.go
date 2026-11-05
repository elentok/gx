package commit

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/commitops"
	"github.com/elentok/gx/ui/diffview"
)

func TestYankLocationOnlyWithYLNoFocusYanksFilePath(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "loc.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.focusDiff = false

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if !strings.Contains(got, "loc.txt") {
		t.Fatalf("expected yl without diff focus to yank file path, got %q", got)
	}
}

func TestYankLocationOnlyWithYLInDiffFocusIncludesLocation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "loc2.txt", "old-1\nold-2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "loc2.txt", "new-1\nnew-2\n")
	testutil.CommitAll(t, repo, "change")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if !strings.Contains(got, "loc2.txt") {
		t.Fatalf("expected yl in diff focus to include filename, got %q", got)
	}
}

func TestYankContentOnlyWithYYInDiffFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "content.txt", "old-1\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "content.txt", "new-1\n")
	testutil.CommitAll(t, repo, "change")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if got == "" {
		t.Fatal("expected yy in diff focus to yank content")
	}
	// Should not contain location info, just the raw line content.
	if strings.Contains(got, "@content.txt") {
		t.Fatalf("expected yy to exclude location header, got %q", got)
	}
}

func TestYankContentOnlyWithYYWithoutDiffFocusWarns(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.focusDiff = false

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	// Clipboard should not have been written (warning returned instead).
	if got != "" {
		t.Fatalf("expected yy without diff focus to not write clipboard, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected warn cmd from yy without diff focus")
	}
}

func TestYankFilenameWithYF(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "a.txt" {
		t.Fatalf("expected yanked filename, got %q", got)
	}
}

func TestYankAllContextWithYAInDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "old-1\nold-2\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "new-1\nnew-2\n")
	testutil.CommitAll(t, repo, "change")

	var got string
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffModel.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	if !strings.Contains(got, "@a.txt L") {
		t.Fatalf("expected ya output to include location, got %q", got)
	}
	if !strings.Contains(got, "-old-1") {
		t.Fatalf("expected ya output to include selected diff line, got %q", got)
	}
}

func TestYankCommitBodyWithYYInHeader(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nline 1\nline 2")

	var got string
	prev := commitops.ClipboardWrite
	commitops.ClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitops.ClipboardWrite = prev })

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.focusHeader = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if got != "line 1\nline 2" {
		t.Fatalf("expected yanked commit body, got %q", got)
	}
}
