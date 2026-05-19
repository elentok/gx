package commit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/nav"
	notifypkg "github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestModel(worktreeRoot, ref string) Model {
	return NewModel(worktreeRoot, ref, "", ui.Settings{UseNerdFontIcons: true}, keys.Manager{})
}

func newTestModelWithFilter(worktreeRoot, ref, filterPath string, settings ui.Settings) Model {
	return NewModel(worktreeRoot, ref, filterPath, settings, keys.Manager{})
}

func TestCurrentRef(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	if m.CurrentRef() != "HEAD" {
		t.Errorf("CurrentRef() = %q, want 'HEAD'", m.CurrentRef())
	}
}

func TestKeyManager(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	km := m.KeyManager()
	_ = km // just verify no panic
}

func TestInputFocused(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	if m.InputFocused() {
		t.Error("expected InputFocused=false initially")
	}
}

func TestNewLoadsCommitDetails(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "first")

	m := newTestModel(repo, "HEAD")
	if m.err != nil {
		t.Fatalf("New err: %v", m.err)
	}
	if m.details.Hash == "" || m.details.Subject != "first" {
		t.Fatalf("unexpected details: %#v", m.details)
	}
}

func TestBToggleBody(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nbody")

	m := newTestModel(repo, "HEAD")
	if !m.bodyExpanded {
		t.Fatalf("expected body expanded by default")
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = updated.(Model)
	if m.bodyExpanded {
		t.Fatalf("expected body collapsed after b")
	}
	m.ready = true
	m.width = 80
	m.height = 20
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "body hidden") {
		t.Fatalf("expected collapsed view without body-hidden hint")
	}
	if !strings.Contains(view, "subject (by ") {
		t.Fatalf("expected collapsed header to show subject line")
	}
	if !strings.Contains(view, "Commit (b to expand)") {
		t.Fatalf("expected collapsed title hint")
	}
}

func TestEnterFocusesDiffAndJKMoveInsideDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "one\nTWO\nTHREE\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.focusDiff {
		t.Fatalf("expected enter to focus diff")
	}
	if m.diffModel.NavMode() != diffview.NavModeHunk {
		t.Fatalf("expected default nav mode hunk, got %v", m.diffModel.NavMode())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.diffModel.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected a to switch to line mode, got %v", m.diffModel.NavMode())
	}

	before := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine <= before {
		t.Fatalf("expected j to move active line, before=%d after=%d", before, m.diffModel.Data().ActiveLine)
	}
}

func TestJKWithoutDiffFocusMoveFiles(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "A\n")
	testutil.WriteFile(t, repo, "b.txt", "B\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	if len(m.files) < 2 {
		t.Fatalf("expected multiple files, got %d", len(m.files))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.fileTreeModel.SelectedIndex() != 1 {
		t.Fatalf("expected j to move selected file, got %d", m.fileTreeModel.SelectedIndex())
	}
	if m.focusDiff {
		t.Fatalf("expected file navigation not to force diff focus")
	}
}

func TestEscLeavesDiffFocusBeforeBackingOut(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected esc in diff to only change focus")
	}
	if m.focusDiff {
		t.Fatalf("expected esc to leave diff focus")
	}
}

func TestTabCyclesBetweenSidebarDiffAndHeader(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "two\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.focusDiff {
		t.Fatalf("expected tab to focus diff")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.focusHeader || m.focusDiff {
		t.Fatalf("expected second tab to focus header")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focusHeader || m.focusDiff {
		t.Fatalf("expected third tab to return focus to files")
	}
}

func TestSidebarSearchMovesToFileMatches(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.WriteFile(t, repo, "beta.txt", "two\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.WriteFile(t, repo, "beta.txt", "TWO\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.search.Mode() != search.SearchModeInput {
		t.Fatalf("expected / to enter search mode")
	}

	for _, r := range []rune("txt") {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if m.search.Query() != "txt" {
		t.Fatalf("search.Query() = %q, want txt", m.search.Query())
	}
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected multiple sidebar matches, got %d", m.search.MatchesCount())
	}
	if m.focusDiff {
		t.Fatalf("expected sidebar search to keep focus in sidebar")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}

	first := m.fileTreeModel.SelectedIndex()
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.fileTreeModel.SelectedIndex() == first {
		t.Fatalf("expected n to move to next sidebar match")
	}
}

func TestFilterPathHighlightsAndFocusesFileOnOpen(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.WriteFile(t, repo, "beta.txt", "two\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.WriteFile(t, repo, "beta.txt", "TWO\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModelWithFilter(repo, "HEAD", "beta.txt", ui.Settings{})
	file, ok := m.selectedCommitFile()
	if !ok {
		t.Fatalf("expected selected commit file")
	}
	if file.Path != "beta.txt" {
		t.Fatalf("selected file = %q, want %q", file.Path, "beta.txt")
	}
	if m.search.Query() != "beta.txt" {
		t.Fatalf("search.Query() = %q, want %q", m.search.Query(), "beta.txt")
	}
	if m.search.Mode() != search.SearchModeNone {
		t.Fatalf("search.Mode() = %v, want SearchModeNone", m.search.Mode())
	}
	if m.searchScope != searchScopeSidebar {
		t.Fatalf("searchScope = %v, want sidebar", m.searchScope)
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected sidebar search matches")
	}
}

func TestFilterPathPassiveSearchDoesNotBlockBack(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "alpha.txt", "one\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "alpha.txt", "ONE\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModelWithFilter(repo, "HEAD", "alpha.txt", ui.Settings{})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected nav back command on q")
	}
	if !nav.IsBack(cmd()) {
		t.Fatalf("expected nav back message")
	}
}

func TestDiffSearchMovesToMatchesAndNavigates(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "alpha\nTWO\nalpha three\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	for _, r := range []rune("alpha") {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected multiple diff matches, got %d", m.search.MatchesCount())
	}
	if !m.focusDiff || m.diffModel.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected diff search to focus diff in line mode")
	}

	first := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine == first {
		t.Fatalf("expected n to move to next diff match")
	}
	second := m.diffModel.Data().ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	m = updated.(Model)
	if m.diffModel.Data().ActiveLine == second {
		t.Fatalf("expected N to move to previous diff match")
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
	prev := commitClipboardWrite
	commitClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { commitClipboardWrite = prev })

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

func TestCommitMessageBodyNormalizesMixedNewlines(t *testing.T) {
	m := Model{
		details: git.CommitDetails{
			Subject: "subject",
			Body:    "subject\r\nline 1\nline 2\rline 3",
		},
	}

	if got := m.commitMessageBody(); got != "line 1\nline 2\nline 3" {
		t.Fatalf("commitMessageBody = %q", got)
	}
}

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

	pane := m.renderFilesPane(30, 10)
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
	entry, ok := m.selectedCommitEntry()
	if !ok {
		t.Fatal("expected selected commit entry")
	}
	if entry.Kind != filetree.EntryFile {
		t.Fatalf("expected initial selection to choose file entry, got %#v", entry)
	}
	if len(m.diffModel.Data().ViewLines) == 0 {
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

	before, ok := m.selectedCommitFile()
	if !ok {
		t.Fatal("expected selected file")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)
	after, ok := m.selectedCommitFile()
	if !ok {
		t.Fatal("expected selected file after .")
	}
	if after.Path == before.Path {
		t.Fatalf("expected . in diff frame to move files, stayed on %q", after.Path)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)
	back, ok := m.selectedCommitFile()
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
	entries := m.fileTreeModel.Entries()
	if len(entries) < 3 {
		t.Fatalf("precondition: expected expanded dir + files, got %d entries", len(entries))
	}
	if entries[0].Kind != filetree.EntryDir || !entries[0].Expanded {
		t.Fatalf("precondition: expected expanded dir row, got kind=%v expanded=%v", entries[0].Kind, entries[0].Expanded)
	}
	m.fileTreeModel.SetSelectedIndex(0)
	return m
}

func TestLeftOnSelectedDirCollapsesCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entries := m.fileTreeModel.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected collapsed tree with 1 dir row, got %d", len(entries))
	}
	if entries[0].Kind != filetree.EntryDir || entries[0].Expanded {
		t.Fatalf("expected collapsed dir row, got kind=%v expanded=%v", entries[0].Kind, entries[0].Expanded)
	}
}

func TestEnterOnSelectedDirCollapsesCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	entries := m.fileTreeModel.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected collapsed tree with 1 dir row after enter, got %d", len(entries))
	}
	if entries[0].Kind != filetree.EntryDir || entries[0].Expanded {
		t.Fatalf("expected collapsed dir row after enter, got kind=%v expanded=%v", entries[0].Kind, entries[0].Expanded)
	}
}

func TestRightOnCollapsedDirExpandsCommitFiletreeDir(t *testing.T) {
	m := commitModelWithDir(t)

	// collapse first
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entries := m.fileTreeModel.Entries()
	if len(entries) != 1 || entries[0].Expanded {
		t.Fatalf("precondition: expected 1 collapsed dir row, got %d entries expanded=%v", len(entries), entries[0].Expanded)
	}

	// expand with l
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	entries = m.fileTreeModel.Entries()
	if len(entries) < 3 {
		t.Fatalf("expected expanded dir + files after l, got %d entries", len(entries))
	}
	if entries[0].Kind != filetree.EntryDir || !entries[0].Expanded {
		t.Fatalf("expected expanded dir row after l, got kind=%v expanded=%v", entries[0].Kind, entries[0].Expanded)
	}
}

func TestRenderDiffPaneShowsActiveHunkMarkerInUnifiedMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, repo, "dir/file.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "dir/file.txt", "one\nTWO\nTHREE\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.focusDiff = true
	m.syncDiffViewport()

	pane := m.renderDiffPane(70, 10)
	if !strings.Contains(pane, "▌") {
		t.Fatalf("expected active hunk marker in diff pane, got:\n%s", pane)
	}
}

func TestViewFitsWindowHeight(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "ONE\nTWO\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24

	view := m.View().Content
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("expected exactly %d lines, got %d", m.height, len(lines))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != m.width {
			t.Fatalf("line %d width: got %d want %d", i, got, m.width)
		}
	}
}

func TestHeaderViewportMaxRowsAndScrollMarkers(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24

	bodyH, _ := m.layoutHeights()
	if got := bodyH - 2; got != 9 {
		t.Fatalf("expected header viewport rows capped at 9 (50%% of height=24), got %d", got)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "↓") && !strings.Contains(view, "") {
		t.Fatalf("expected bottom overflow marker at top of header scroll")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'J', Text: "J", ShiftedCode: 'J'})
	m = updated.(Model)
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "↑") && !strings.Contains(view, "") {
		t.Fatalf("expected top overflow marker after scrolling header")
	}
}

func TestHeaderScrollRequiresHeaderFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "subject\n\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24

	before := m.headerOffset
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.headerOffset != before {
		t.Fatalf("expected j without header focus not to scroll header")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.focusHeader {
		t.Fatalf("expected header focus after second tab")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.headerOffset <= before {
		t.Fatalf("expected j with header focus to scroll header")
	}
}

func TestCMOutsideDiffDoesNothing(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "one\n")
	testutil.CommitAll(t, repo, "base")

	m := newTestModel(repo, "HEAD")
	m.focusDiff = false

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("first c should not run command")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("cm outside diff should not run command")
	}
}

func TestCMInDiffWithoutFileContextShowsError(t *testing.T) {
	repo := testutil.TempRepo(t)
	m := newTestModel(repo, "HEAD")
	m.focusDiff = true
	m.files = nil
	m.fileTreeModel.SetEntries(nil)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	_ = m
	if cmd == nil {
		t.Fatalf("cm without file context should return a notify cmd")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok || notifyMsg.Message != "no file context for comment" {
		t.Fatalf("expected notify msg %q, got %T %+v", "no file context for comment", msg, msg)
	}
}

func TestMouseWheelScrollsDiffViewport(t *testing.T) {
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(after, "\n")+"\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	bodyH, contentH := m.layoutHeights()
	diffX := m.filesPaneWidth(contentH) + 1
	diffY := bodyH + 1

	beforeOffset := m.diffModel.Viewport().YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: diffX, Y: diffY, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffModel.Viewport().YOffset() <= beforeOffset {
		t.Fatalf("expected diff viewport to scroll down on wheel, before=%d after=%d", beforeOffset, m.diffModel.Viewport().YOffset())
	}
}

func TestMouseWheelOverFiletreeScrollsFiletree(t *testing.T) {
	repo := testutil.TempRepo(t)
	// Create and then change enough files so the filetree can scroll
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "original")
	}
	testutil.CommitAll(t, repo, "add files")
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "changed")
	}
	testutil.CommitAll(t, repo, "change all")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	bodyH, _ := m.layoutHeights()
	filetreeX := 1
	filetreeY := bodyH + 1

	beforeOffset := m.fileTreeModel.ScrollOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: filetreeX, Y: filetreeY, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.fileTreeModel.ScrollOffset() <= beforeOffset {
		t.Fatalf("expected filetree to scroll down on wheel over filetree, before=%d after=%d", beforeOffset, m.fileTreeModel.ScrollOffset())
	}
}

func TestMouseWheelOutsideDiffDoesNotScroll(t *testing.T) {
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(after, "\n")+"\n")
	testutil.CommitAll(t, repo, "change")

	m := newTestModel(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	beforeOffset := m.diffModel.Viewport().YOffset()
	// scroll over the file sidebar (x=1 is in the sidebar for wide layout)
	updated, _ := m.Update(tea.MouseWheelMsg{X: 1, Y: 10, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffModel.Viewport().YOffset() != beforeOffset {
		t.Fatalf("expected diff viewport not to scroll when wheel is over sidebar, before=%d after=%d", beforeOffset, m.diffModel.Viewport().YOffset())
	}
}
