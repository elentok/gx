package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesPaneShowsNerdFontIcons(t *testing.T) {
	repo := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, repo, "dir/file.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "dir/file.txt", "two\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24

	pane := m.sidebar.renderFilesPane(m.filesInput(), 30, 10)
	icons := ui.Icons(true)
	if !strings.Contains(pane, icons.FolderOpen) {
		t.Fatalf("expected files pane to show folder icon, got:\n%s", pane)
	}
	if !strings.Contains(pane, icons.FileModified) {
		t.Fatalf("expected files pane to show file icon, got:\n%s", pane)
	}
}

func TestInitialSelectionChoosesFirstFileOverDirectory(t *testing.T) {
	repo := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, repo, "dir/file.txt", "one\ntwo\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "dir/file.txt", "ONE\nTWO\nTHREE\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	entry, ok := m.sidebar.selectedCommitEntry()
	if !ok {
		t.Fatal("expected selected commit entry")
	}
	if entry.HasChildren {
		t.Fatalf("expected initial selection to choose file entry, got %#v", entry)
	}
	if len(m.diff.diffModel.Data().ViewLines) == 0 {
		t.Fatal("expected initial file selection to load diff")
	}
}

func TestCommaDotInFilesFrameSwitchCommits(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "middle")
	testutil.WriteFile(t, repo, "a.txt", "three\n")
	testutil.CommitAll(t, repo, "top")

	m := newTestModel(repo, "HEAD~1")
	m.ready = true

	if m.details.Subject != "middle" {
		t.Fatalf("expected initial middle commit, got %q", m.details.Subject)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	if m.details.Subject != "top" {
		t.Fatalf("expected . in files frame to move to newer commit, got %q", m.details.Subject)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)
	if m.details.Subject != "middle" {
		t.Fatalf("expected , in files frame to move back to previous commit, got %q", m.details.Subject)
	}
}

func TestCommaDotInHeaderFrameSwitchCommits(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "middle")
	testutil.WriteFile(t, repo, "a.txt", "three\n")
	testutil.CommitAll(t, repo, "top")

	m := newTestModel(repo, "HEAD~1")
	m.ready = true
	m.focusHeader = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	if m.details.Subject != "top" {
		t.Fatalf("expected . in header frame to move newer, got %q", m.details.Subject)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)
	if m.details.Subject != "middle" {
		t.Fatalf("expected , in header frame to move older, got %q", m.details.Subject)
	}
}

func TestCommaDotInDiffFrameSwitchFiles(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "A\n")
	testutil.WriteFile(t, repo, "b.txt", "B\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true

	before, ok := m.sidebar.selectedCommitFile()
	if !ok {
		t.Fatal("expected selected file")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	after, ok := m.sidebar.selectedCommitFile()
	if !ok {
		t.Fatal("expected selected file after .")
	}
	if after.Path == before.Path {
		t.Fatalf("expected . in diff frame to move files, stayed on %q", after.Path)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)
	back, ok := m.sidebar.selectedCommitFile()
	if !ok {
		t.Fatal("expected selected file after ,")
	}
	if back.Path != before.Path {
		t.Fatalf("expected , in diff frame to return to %q, got %q", before.Path, back.Path)
	}
}

// commitModelWithDir creates a commit model with HEAD pointing to a commit
// that modified two files inside "dir/". The dir row is pre-selected.
// Precondition asserts the dir is expanded so callers start from a known state.
func commitModelWithDir(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, repo, "dir/a.txt", "one\n")
	testutil.WriteFile(t, repo, "dir/b.txt", "two\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "dir/a.txt", "ONE\n")
	testutil.WriteFile(t, repo, "dir/b.txt", "TWO\n")
	testutil.CommitAll(t, repo, "change")
	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	entries := m.sidebar.fileTreeModel.Entries()
	if len(entries) < 3 {
		t.Fatalf("precondition: expected expanded dir + files, got %d entries", len(entries))
	}
	if !entries[0].HasChildren || !entries[0].Expanded {
		t.Fatalf("precondition: expected expanded dir row, got hasChildren=%v expanded=%v", entries[0].HasChildren, entries[0].Expanded)
	}
	m.sidebar.fileTreeModel.SetSelectedIndex(0)
	return m
}

func TestLeftOnSelectedDirCollapsesCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entries := m.sidebar.fileTreeModel.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected collapsed tree with 1 dir row, got %d", len(entries))
	}
	if !entries[0].HasChildren || entries[0].Expanded {
		t.Fatalf("expected collapsed dir row, got hasChildren=%v expanded=%v", entries[0].HasChildren, entries[0].Expanded)
	}
}

func TestEnterOnSelectedDirCollapsesCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	entries := m.sidebar.fileTreeModel.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected collapsed tree with 1 dir row after enter, got %d", len(entries))
	}
	if !entries[0].HasChildren || entries[0].Expanded {
		t.Fatalf("expected collapsed dir row after enter, got hasChildren=%v expanded=%v", entries[0].HasChildren, entries[0].Expanded)
	}
}

func TestRightOnCollapsedDirExpandsCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	// collapse first
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entries := m.sidebar.fileTreeModel.Entries()
	if len(entries) != 1 || entries[0].Expanded {
		t.Fatalf("precondition: expected 1 collapsed dir row, got %d entries expanded=%v", len(entries), entries[0].Expanded)
	}

	// expand with l
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	entries = m.sidebar.fileTreeModel.Entries()
	if len(entries) < 3 {
		t.Fatalf("expected expanded dir + files after l, got %d entries", len(entries))
	}
	if !entries[0].HasChildren || !entries[0].Expanded {
		t.Fatalf("expected expanded dir row after l, got hasChildren=%v expanded=%v", entries[0].HasChildren, entries[0].Expanded)
	}
}
