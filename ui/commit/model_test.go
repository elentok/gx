package commit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNewLoadsCommitDetails(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.CommitAll(t, repo, "first")

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.focusDiff {
		t.Fatalf("expected enter to focus diff")
	}
	if m.diffNavMode != explorer.NavHunk {
		t.Fatalf("expected default nav mode hunk, got %v", m.diffNavMode)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.diffNavMode != explorer.NavLine {
		t.Fatalf("expected a to switch to line mode, got %v", m.diffNavMode)
	}

	before := m.section.ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.section.ActiveLine <= before {
		t.Fatalf("expected j to move active line, before=%d after=%d", before, m.section.ActiveLine)
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

	m := New(repo, "HEAD")
	m.ready = true
	if len(m.files) < 2 {
		t.Fatalf("expected multiple files, got %d", len(m.files))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.selected != 1 {
		t.Fatalf("expected j to move selected file, got %d", m.selected)
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

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.searchMode != searchModeInput {
		t.Fatalf("expected / to enter search mode")
	}

	for _, r := range []rune("txt") {
		updated, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}
	if m.searchQuery != "txt" {
		t.Fatalf("searchQuery = %q, want txt", m.searchQuery)
	}
	if len(m.fileMatches) < 2 {
		t.Fatalf("expected multiple sidebar matches, got %d", len(m.fileMatches))
	}
	if m.focusDiff {
		t.Fatalf("expected sidebar search to keep focus in sidebar")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.searchMode != searchModeNone {
		t.Fatalf("expected enter to leave search mode")
	}

	first := m.selected
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.selected == first {
		t.Fatalf("expected n to move to next sidebar match")
	}
}

func TestDiffSearchMovesToMatchesAndNavigates(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "a.txt", "alpha\nTWO\nalpha three\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := New(repo, "HEAD")
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
	if len(m.searchMatches) < 2 {
		t.Fatalf("expected multiple diff matches, got %d", len(m.searchMatches))
	}
	if !m.focusDiff || m.diffNavMode != explorer.NavLine {
		t.Fatalf("expected diff search to focus diff in line mode")
	}

	first := m.section.ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.section.ActiveLine == first {
		t.Fatalf("expected n to move to next diff match")
	}
	second := m.section.ActiveLine
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	m = updated.(Model)
	if m.section.ActiveLine == second {
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

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24
	m.syncDiffViewport()
	m.focusDiff = true
	m.diffNavMode = explorer.NavLine

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

	m := New(repo, "HEAD")
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

	m := NewWithSettings(repo, "HEAD", Settings{UseNerdFontIcons: true})
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

	m := New(repo, "HEAD")
	entry, ok := m.selectedCommitEntry()
	if !ok {
		t.Fatal("expected selected commit entry")
	}
	if entry.Kind != commitFileEntryFile {
		t.Fatalf("expected initial selection to choose file entry, got %#v", entry)
	}
	if len(m.section.ViewLines) == 0 {
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

	m := New(repo, "HEAD~1")
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

	m := New(repo, "HEAD~1")
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

	m := New(repo, "HEAD")
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

func TestRenderDiffPaneShowsActiveHunkMarkerInUnifiedMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	testutil.WriteFile(t, repo, "dir/file.txt", "one\ntwo\nthree\nfour\n")
	testutil.CommitAll(t, repo, "base")
	testutil.WriteFile(t, repo, "dir/file.txt", "one\nTWO\nTHREE\nfour\n")
	testutil.CommitAll(t, repo, "change")

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
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

	m := New(repo, "HEAD")
	m.ready = true
	m.width = 100
	m.height = 24

	bodyH, _ := m.layoutHeights()
	if got := bodyH - 2; got != 6 {
		t.Fatalf("expected header viewport rows capped at 6, got %d", got)
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

	m := New(repo, "HEAD")
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
