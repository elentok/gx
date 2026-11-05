package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/status/diffarea"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpaceStagesSingleLineInLineMode(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "line-1\nline-2\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	m.applySelection()

	staged, err := stage.DiffPath(repo, "line.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected staged diff after line-mode space")
	}
}

func TestFiletreeSpaceTogglesWholeFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected file to be staged by filetree space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err = stage.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached after unstage: %v", err)
	}
	if staged != "" {
		t.Fatalf("expected file to be unstaged by second filetree space")
	}
}

func TestFiletreeDDiscardsUntrackedFileAfterConfirm(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "new\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if !m.modals.confirmOpen {
		t.Fatalf("expected discard confirmation to open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if _, err := os.Stat(repo + "/new.txt"); err == nil {
		t.Fatalf("expected untracked file to be deleted after discard confirm")
	}
	if len(m.statusData.statusEntries) != 0 {
		t.Fatalf("expected no status entries after discard, got %d", len(m.statusData.statusEntries))
	}
}

func TestFiletreeDDiscardsDirectoryAfterConfirm(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/dir")
	testutil.WriteFile(t, repo, "dir/tracked.txt", "original\n")
	testutil.MustGitExported(t, repo, "add", "dir/tracked.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "dir/tracked.txt", "changed\n")
	testutil.WriteFile(t, repo, "dir/new.txt", "new\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "dir" {
		t.Fatalf("expected selected directory entry, got %+v ok=%v", entry, ok)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if !m.modals.confirmOpen {
		t.Fatalf("expected discard confirmation to open for directory")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	content, err := os.ReadFile(filepath.Join(repo, "dir/tracked.txt"))
	if err != nil {
		t.Fatalf("expected tracked file to remain after directory discard: %v", err)
	}
	if string(content) != "original\n" {
		t.Fatalf("expected tracked file to be restored, got %q", string(content))
	}
	if _, err := os.Stat(filepath.Join(repo, "dir/new.txt")); err == nil {
		t.Fatalf("expected untracked file inside directory to be deleted after discard confirm")
	}
	if len(m.statusData.statusEntries) != 0 {
		t.Fatalf("expected no status entries after directory discard, got %d", len(m.statusData.statusEntries))
	}
}

func TestDiffUnstagedDDiscardsLineAfterConfirm(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "line.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "line.txt", "ONE\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)
	if !m.modals.confirmOpen {
		t.Fatalf("expected discard confirmation in unstaged diff")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	unstaged, err := stage.DiffPath(repo, "line.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath unstaged: %v", err)
	}
	if strings.Contains(unstaged, "+ONE") || !strings.Contains(unstaged, "-one") {
		t.Fatalf("expected selected +line to be discarded from worktree, got:\n%s", unstaged)
	}
}

func TestDiffStagedDUnstagesSelectionWithoutConfirm(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "one\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionStaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = updated.(Model)

	if m.modals.confirmOpen {
		t.Fatalf("did not expect discard confirm in staged diff; d should unstage")
	}
	unstaged, err := stage.DiffPath(repo, "line.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath unstaged: %v", err)
	}
	if strings.TrimSpace(unstaged) == "" {
		t.Fatalf("expected unstaged diff after using d in staged view")
	}
}

func TestApplySelection_DoesNotSwitchSectionWhenHunksRemain(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "l01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nl20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "README.md", "L01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nL20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.diffarea.Unstaged.DataRef().ActiveHunk = 0
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) < 2 {
		t.Fatalf("expected at least 2 hunks before apply, got %d", len(m.diffarea.Unstaged.DataRef().Parsed.Hunks))
	}

	cmd := m.applySelection()
	if cmd != nil {
		// animation may or may not be set; ignore command
	}
	if m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("section switched unexpectedly while hunks remain: got=%v", m.diffarea.ActiveSection)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) == 0 {
		t.Fatalf("expected unstaged hunks to remain after staging first hunk")
	}
}

func TestApplySelection_DoesNotSwitchSectionWhenCurrentSectionBecomesEmpty(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "one\ntwo\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "README.md", "ONE\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)

	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) != 1 {
		t.Fatalf("expected exactly 1 unstaged hunk before apply, got %d", len(m.diffarea.Unstaged.DataRef().Parsed.Hunks))
	}

	cmd := m.applySelection()
	if cmd != nil {
		// animation may or may not be set; ignore command
	}
	if m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("section switched unexpectedly after source became empty: got=%v", m.diffarea.ActiveSection)
	}
}

func TestApplySelection_FlashesDestinationSectionWithoutSwitching(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "one\ntwo\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "README.md", "ONE\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)

	_ = m.applySelection()

	if m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("section switched unexpectedly: got=%v", m.diffarea.ActiveSection)
	}
	if !m.diffarea.Flash.Active {
		t.Fatalf("expected destination flash to be active")
	}
	if m.diffarea.Flash.Section != diffarea.SectionStaged {
		t.Fatalf("expected flash on staged destination, got=%v", m.diffarea.Flash.Section)
	}
}

func TestLineModeCanUnstageSingleModifiedLine(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "README.md", "new-1\nnew-2\nnew-3\n")

	m := newTestModelDefault(repo)
	m.ready = true

	// Stage everything first.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	// Enter diff view and switch to line mode.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)

	// Move to second changed line and unstage it.
	for range 4 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	unstaged, err := stage.DiffPath(repo, "README.md", false, 1)
	if err != nil {
		t.Fatalf("unstaged diff: %v", err)
	}

	if !strings.Contains(staged, "+new-1") || strings.Contains(staged, "+new-2") || !strings.Contains(unstaged, "+new-2") {
		t.Fatalf("unexpected diffs after unstage line\nSTAGED:\n%s\nUNSTAGED:\n%s", staged, unstaged)
	}
}

func TestLineModeStagesSingleLineInUntrackedFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "line-1\nline-2\nline-3\n")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "new.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+line-1") || !strings.Contains(staged, "+line-2") || strings.Contains(staged, "+line-3") {
		t.Fatalf("expected single line staged for untracked file\nSTAGED:\n%s", staged)
	}
}

func TestLineModeUnstageOneOfAdjacentDeletedLinesDoesNotDuplicate(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	base := strings.Join([]string{
		"func f() {",
		"    if cond {",
		"        x()",
		"    }",
		"    y()",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "f.go", base)
	testutil.MustGitExported(t, repo, "add", "f.go")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := strings.Join([]string{
		"func f() {",
		"    if cond {",
		"    y()",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "f.go", updated)

	m := newTestModelDefault(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := stage.DiffPath(repo, "f.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated closing brace in staged diff:\n%s", staged)
	}
}

func TestLineModeUnstageBraceFromFirstHunkBlock(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	base := strings.Join([]string{
		"package git",
		"",
		"func DiffUntrackedPath(worktreeRoot, path string, color bool, contextLines int) (string, error) {",
		"    diffPath := path",
		"    absPath := path",
		"    if !filepath.IsAbs(path) {",
		"        absPath = filepath.Join(worktreeRoot, path)",
		"    }",
		"",
		"    if !color {",
		"        return \"\", nil",
		"    }",
		"    _ = absPath",
		"    return \"\", nil",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "stage.go", base)
	testutil.MustGitExported(t, repo, "add", "stage.go")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := strings.Join([]string{
		"package git",
		"",
		"func DiffUntrackedPath(worktreeRoot, path string, color bool, contextLines int) (string, error) {",
		"    diffPath := path",
		"",
		"    if !color {",
		"        return \"\", nil",
		"    }",
		"    return \"\", nil",
		"}",
	}, "\n") + "\n"
	testutil.WriteFile(t, repo, "stage.go", updated)

	m := newTestModelDefault(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updatedModel.(Model)

	// Move to the deletion of the closing brace line.
	for i := 0; i < 3; i++ {
		updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updatedModel.(Model)
	}
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := stage.DiffPath(repo, "stage.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated brace in staged diff:\n%s", staged)
	}
}

func TestVisualModeStagesLineRange(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\nthree\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	if !m.diffarea.Unstaged.DataRef().VisualActive {
		t.Fatalf("expected visual mode active after v")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "range.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if !strings.Contains(staged, "+one") || !strings.Contains(staged, "+two") {
		t.Fatalf("expected staged diff to include selected visual range:\n%s", staged)
	}
	if m.diffarea.Unstaged.DataRef().VisualActive {
		t.Fatalf("expected visual mode to exit after applying selection")
	}
}

func TestEscExitsVisualModeAndKeepsDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	if !m.diffarea.Unstaged.DataRef().VisualActive {
		t.Fatalf("expected visual mode active after v")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)

	if m.focus != focusDiff {
		t.Fatalf("expected esc in visual mode to keep diff focus")
	}
	if m.diffarea.Unstaged.DataRef().VisualActive {
		t.Fatalf("expected esc to exit visual mode")
	}
}

func TestDiffDotMovesToNextFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	before, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file before navigation")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)

	after, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file after navigation")
	}
	if after.Path == before.Path {
		t.Fatalf("expected '.' to move to next file, stayed on %q", after.Path)
	}
	if m.focus != focusDiff {
		t.Fatalf("expected to remain in diff focus, got %v", m.focus)
	}
}

func TestDiffCommaMovesToPreviousFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	first, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '.', Text: "."})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = updated.(Model)

	back, ok := m.selectedFile()
	if !ok {
		t.Fatalf("expected selected file after ','")
	}
	if back.Path != first.Path {
		t.Fatalf("expected ',' to return to previous file %q, got %q", first.Path, back.Path)
	}
}

func TestVisualModeUnstagesLineRange(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "range.txt", "one\ntwo\nthree\n")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionStaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "range.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath(staged): %v", err)
	}
	unstaged, err := stage.DiffPath(repo, "range.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath(unstaged): %v", err)
	}

	if strings.Contains(staged, "+one") || strings.Contains(staged, "+two") || !strings.Contains(staged, "+three") {
		t.Fatalf("expected staged diff to keep only third line:\n%s", staged)
	}
	if !strings.Contains(unstaged, "+one") || !strings.Contains(unstaged, "+two") {
		t.Fatalf("expected unstaged diff to include selected visual range:\n%s", unstaged)
	}
}
