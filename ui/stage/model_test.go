package stage

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gx/git"
	"gx/testutil"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUseStackedLayoutThreshold(t *testing.T) {
	m := Model{width: 100}
	if !m.useStackedLayout() {
		t.Fatal("expected stacked layout at width 100")
	}
	m.width = 101
	if m.useStackedLayout() {
		t.Fatal("expected side-by-side layout at width 101")
	}
}

func TestQAndEscFocusBehavior(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected nil cmd on esc")
	}
	m2 := updated.(Model)
	if m2.focus != focusStatus {
		t.Fatalf("esc should move focus to status")
	}

	m2.focus = focusStatus
	updated, cmd = m2.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in status should quit")
	}
}

func TestQAlwaysQuitsFromDiffFocus(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected q to quit from diff focus")
	}
}

func TestStatusLOnFileEntersDiffAndResetsSectionOnFileChange(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.WriteFile(t, repo, "b.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt", "b.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// File a has both staged and unstaged changes.
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	// File b is only unstaged.
	testutil.WriteFile(t, repo, "b.txt", "ONE\ntwo\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.section = sectionStaged

	if len(m.statusEntries) < 2 {
		t.Fatalf("expected two status entries, got %d", len(m.statusEntries))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff {
		t.Fatalf("expected l on file to enter diff")
	}
	if m.section != sectionStaged {
		t.Fatalf("expected section to remain staged for same file")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if m.focus != focusStatus {
		t.Fatalf("expected h in diff to return to status")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.section != sectionUnstaged {
		t.Fatalf("expected section reset to unstaged after active file change")
	}
}

func TestHelpOverlayToggleAndCompactStatusBar(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 40
	m.focus = focusStatus

	line := m.helpLine()
	if !strings.Contains(line, "? help") || strings.Contains(line, "j/k") {
		t.Fatalf("expected compact status help line, got %q", line)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if !m.helpOpen {
		t.Fatalf("expected help overlay to open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if !m.helpOpen {
		t.Fatalf("expected help overlay to stay open while scrolling")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.helpOpen {
		t.Fatalf("expected help overlay to close on esc")
	}
}

func TestHelpLineRightAlignsHintAndTruncatesStatus(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.width = 48
	m.focus = focusStatus
	m.statusMsg = "this is a very long status message that should truncate"

	line := m.helpLine()
	plain := ansi.Strip(line)

	if ansi.StringWidth(plain) != m.width {
		t.Fatalf("expected footer width %d, got %d (%q)", m.width, ansi.StringWidth(plain), plain)
	}
	if !strings.Contains(plain, "...") {
		t.Fatalf("expected truncated status with ellipsis, got %q", plain)
	}
	if !strings.HasSuffix(plain, "· status · ? help") {
		t.Fatalf("expected hint right-aligned at end, got %q", plain)
	}
}

func TestRefreshesOnFocusMsg(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	m := New(repo)
	m.ready = true

	countBefore := len(m.statusEntries)
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(Model)

	if len(m.statusEntries) <= countBefore {
		t.Fatalf("expected refresh on focus to include new file; before=%d after=%d", countBefore, len(m.statusEntries))
	}
}

func TestViewEnablesReportFocus(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	v := m.View()
	if !v.ReportFocus {
		t.Fatalf("expected ReportFocus enabled on stage view")
	}
}

func TestSpaceStagesSingleLineInLineMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "line-1\nline-2\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	m.applySelection()

	staged, err := git.DiffPath(repo, "line.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected staged diff after line-mode space, status=%q", m.statusMsg)
	}
}

func TestStatusSpaceTogglesWholeFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected file to be staged by status space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err = git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached after unstage: %v", err)
	}
	if staged != "" {
		t.Fatalf("expected file to be unstaged by second status space")
	}
}

func TestDiffJKScrollsWithoutMovingCursor(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	beforeLine := m.unstaged.activeLine
	beforeOffset := m.unstaged.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("J changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	expectedDelta := 3
	if beforeOffset+expectedDelta > maxOffset {
		expectedDelta = maxOffset - beforeOffset
	}
	if expectedDelta < 0 {
		expectedDelta = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != beforeOffset+expectedDelta {
		t.Fatalf("J should scroll by up to 3: before=%d after=%d expected=%d", beforeOffset, got, beforeOffset+expectedDelta)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("K changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
	if expectedDelta > 0 {
		if got := m.unstaged.viewport.YOffset(); got >= beforeOffset+expectedDelta {
			t.Fatalf("K should scroll up on first press: offset after K=%d", got)
		}
	}
}

func TestHunkModeJKScrollsLargeHunkBeforeSwitching(t *testing.T) {
	repo := testutil.TempRepo(t)

	base := make([]string, 0, 48)
	for i := 1; i <= 48; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 20; i++ {
		updated[i] = "new-" + updated[i]
	}
	for i := 34; i < 38; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()

	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected first hunk active initially, got %d", m.unstaged.activeHunk)
	}
	beforeOffset := m.unstaged.viewport.YOffset()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected j to scroll large hunk before switching, activeHunk=%d", m.unstaged.activeHunk)
	}
	if m.unstaged.viewport.YOffset() <= beforeOffset {
		t.Fatalf("expected j to scroll down within large hunk")
	}

	midOffset := m.unstaged.viewport.YOffset()
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updatedModel.(Model)
	if m.unstaged.activeHunk != 0 {
		t.Fatalf("expected k to scroll large hunk before switching, activeHunk=%d", m.unstaged.activeHunk)
	}
	if m.unstaged.viewport.YOffset() >= midOffset {
		t.Fatalf("expected k to scroll up within large hunk")
	}
}

func TestHunkOverflowViewportMarkers(t *testing.T) {
	repo := testutil.TempRepo(t)

	base := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 24; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	assertMarkers := func(useNerd bool, up, down string) {
		t.Helper()
		m := NewWithSettings(repo, Settings{DiffContextLines: 1, UseNerdFontIcons: useNerd})
		m.ready = true
		m.width = 100
		m.height = 16
		m.focus = focusDiff
		m.section = sectionUnstaged
		m.navMode = navHunk
		m.syncDiffViewports()

		m.unstaged.viewport.SetYOffset(3)
		pane := m.renderSectionPane(80, 10, "Unstaged", &m.unstaged, sectionUnstaged)

		if strings.Contains(pane, "hunk>view") {
			t.Fatalf("unexpected legacy hunk>view indicator in pane:\n%s", pane)
		}
		if !strings.Contains(pane, up) || !strings.Contains(pane, down) {
			t.Fatalf("expected overflow markers %q and %q in pane:\n%s", up, down, pane)
		}
	}

	assertMarkers(true, "", "")
	assertMarkers(false, "↑", "↓")
}

func TestGGAndGInStatusAndDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")
	testutil.WriteFile(t, repo, "c.txt", "three\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 1

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.selected != 0 {
		t.Fatalf("expected gg to jump status selection to top, got %d", m.selected)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected G to jump status selection to bottom, got %d", m.selected)
	}

	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine
	if len(m.unstaged.parsed.Changed) == 0 {
		t.Fatalf("expected unstaged changes in diff view")
	}
	m.unstaged.activeLine = len(m.unstaged.parsed.Changed) - 1

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.unstaged.activeLine != 0 {
		t.Fatalf("expected gg to jump active diff line to top, got %d", m.unstaged.activeLine)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.unstaged.activeLine != len(m.unstaged.parsed.Changed)-1 {
		t.Fatalf("expected G to jump active diff line to bottom, got %d", m.unstaged.activeLine)
	}
}

func TestUppercaseGUsingShiftedCodeJumpsBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected shifted G to jump to bottom, got %d", m.selected)
	}
}

func TestUppercaseGUsingShiftModifierJumpsBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus
	m.selected = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g", Mod: tea.ModShift})
	m = updated.(Model)
	if m.selected != len(m.statusEntries)-1 {
		t.Fatalf("expected shifted modifier G to jump to bottom, got %d", m.selected)
	}
}

func TestGInDiffHunkModeJumpsViewportToBottom(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		base = append(base, fmt.Sprintf("line-%02d", i))
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	updated := append([]string{}, base...)
	for i := 0; i < 28; i++ {
		updated[i] = "new-" + updated[i]
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	m := New(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updatedModel.(Model)

	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != maxOffset {
		t.Fatalf("expected G to jump diff viewport to bottom, got %d want %d", got, maxOffset)
	}
}

func TestCtrlDAndCtrlUScrollStatusAndDiff(t *testing.T) {
	repo := testutil.TempRepo(t)
	for i := 0; i < 16; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("f%02d.txt", i), "x\n")
	}

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.focus = focusStatus
	beforeSel := m.selected

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected <= beforeSel {
		t.Fatalf("expected ctrl+d to move status selection down")
	}

	midSel := m.selected
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.selected >= midSel {
		t.Fatalf("expected ctrl+u to move status selection up")
	}

	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.syncDiffViewports()
	beforeOffset := m.unstaged.viewport.YOffset()

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.unstaged.viewport.YOffset() < beforeOffset {
		t.Fatalf("expected ctrl+d to scroll diff down")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.unstaged.viewport.YOffset() > beforeOffset {
		t.Fatalf("expected ctrl+u to scroll diff up")
	}
}

func TestStatusFileIconDeletedAndFallback(t *testing.T) {
	deleted := git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'}

	nerd := statusPaneIconsFor(true)
	if got := statusFileIcon(deleted, nerd); got != "" {
		t.Fatalf("expected deleted nerd icon, got %q", got)
	}

	plain := statusPaneIconsFor(false)
	if got := statusFileIcon(deleted, plain); got != "D" {
		t.Fatalf("expected deleted fallback icon, got %q", got)
	}
}

func TestStatusEntryColorDeletedFileIsDim(t *testing.T) {
	entry := statusEntry{
		Kind: statusEntryFile,
		File: git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'},
	}
	if got := statusEntryColor(entry); got != "#a6adc8" {
		t.Fatalf("expected dim deleted color, got %q", got)
	}
}

func TestStatusMessageClearsAfterTimeoutTick(t *testing.T) {
	m := New(testutil.TempRepo(t))
	m.ready = true
	m.statusMsg = "temporary"
	m.statusUntil = time.Now().Add(-time.Second)

	updated, _ := m.Update(statusTickMsg{})
	m = updated.(Model)
	if m.statusMsg != "" {
		t.Fatalf("expected status message to clear after timeout tick")
	}
}

func TestStatusSelectionDebouncesDiffReload(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := New(repo)
	m.ready = true
	m.focus = focusStatus

	before := m.activeFilePath
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected j in status to schedule debounced reload")
	}
	if m.activeFilePath != before {
		t.Fatalf("expected active file to remain unchanged before debounce fires")
	}

	updated, _ = m.Update(diffReloadMsg{seq: m.diffReloadSeq})
	m = updated.(Model)
	if m.activeFilePath == before {
		t.Fatalf("expected active file to update after debounce message")
	}
}

func TestDiffJDoesNotOverscrollPastContent(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged

	for i := 0; i < 300; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
		m = updated.(Model)
	}

	maxOffset := m.unstaged.viewport.TotalLineCount() - m.unstaged.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if got := m.unstaged.viewport.YOffset(); got != maxOffset {
		t.Fatalf("overscrolled: got offset=%d want=%d", got, maxOffset)
	}
}

func TestApplySelection_DoesNotSwitchSectionWhenHunksRemain(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "l01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nl20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")
	testutil.CommitAll(t, repo, "baseline")
	testutil.WriteFile(t, repo, "README.md", "L01\nl02\nl03\nl04\nl05\nl06\nl07\nl08\nl09\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nL20\nl21\nl22\nl23\nl24\nl25\nl26\nl27\nl28\nl29\nl30\n")

	m := New(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navHunk
	m.unstaged.activeHunk = 0
	if len(m.unstaged.parsed.Hunks) < 2 {
		t.Fatalf("expected at least 2 hunks before apply, got %d", len(m.unstaged.parsed.Hunks))
	}

	cmd := m.applySelection()
	if cmd != nil {
		// animation may or may not be set; ignore command
	}
	if m.section != sectionUnstaged {
		t.Fatalf("section switched unexpectedly while hunks remain: got=%v", m.section)
	}
	if len(m.unstaged.parsed.Hunks) == 0 {
		t.Fatalf("expected unstaged hunks to remain after staging first hunk")
	}
}

func TestCCTriggersCommitCommand(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := New(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)
	if m.keyPrefix != "c" {
		t.Fatalf("expected keyPrefix=c after first c, got %q", m.keyPrefix)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatalf("second c should launch commit command")
	}
	m = updated.(Model)
	if m.keyPrefix != "" {
		t.Fatalf("expected keyPrefix reset after cc, got %q", m.keyPrefix)
	}
}

func TestWToggleSoftWrap(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "this is a very long line that should wrap in narrow diff panes\n")

	m := New(repo)
	m.ready = true
	m.width = 80
	m.height = 20
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.syncDiffViewports()

	wrappedCount := len(m.unstaged.viewLines)
	if !m.wrapSoft {
		t.Fatal("expected wrapSoft enabled by default")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if m.wrapSoft {
		t.Fatal("expected wrapSoft disabled after w")
	}
	unwrappedCount := len(m.unstaged.viewLines)
	if unwrappedCount > wrappedCount {
		t.Fatalf("expected unwrapped lines <= wrapped lines, got wrapped=%d unwrapped=%d", wrappedCount, unwrappedCount)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if !m.wrapSoft {
		t.Fatal("expected wrapSoft enabled after second w")
	}
}

func TestDefaultSettingsEnableNerdFontIcons(t *testing.T) {
	settings := DefaultSettings()
	if !settings.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestStatusEntryColor(t *testing.T) {
	tests := []struct {
		name  string
		entry statusEntry
		want  string
	}{
		{name: "unstaged", entry: statusEntry{HasUnstaged: true}, want: "#cdd6f4"},
		{name: "partial", entry: statusEntry{HasStaged: true, HasUnstaged: true}, want: "#fab387"},
		{name: "staged", entry: statusEntry{HasStaged: true}, want: "#a6e3a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusEntryColor(tt.entry); got != tt.want {
				t.Fatalf("statusEntryColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEntryMeta_NerdFont(t *testing.T) {
	icons := statusPaneIconsFor(true)

	if got := statusEntryMeta(statusEntry{HasStaged: true, HasUnstaged: true}, true, icons); got != "" {
		t.Fatalf("partial nerd icon = %q", got)
	}
	if got := statusEntryMeta(statusEntry{HasStaged: true}, true, icons); got != "" {
		t.Fatalf("staged nerd icon = %q", got)
	}
}

func TestStatusFileIcon(t *testing.T) {
	icons := statusPaneIconsFor(true)

	if got := statusFileIcon(git.StageFileStatus{IndexStatus: '?', WorktreeCode: '?'}, icons); got != "" {
		t.Fatalf("untracked icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: 'A', WorktreeCode: ' '}, icons); got != "" {
		t.Fatalf("added icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: ' ', WorktreeCode: 'M'}, icons); got != "" {
		t.Fatalf("modified icon = %q, want modified file icon", got)
	}
}

func TestLineModeCanUnstageSingleModifiedLine(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "README.md", "new-1\nnew-2\nnew-3\n")

	m := New(repo)
	m.ready = true

	// Stage everything first.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	// Enter diff view and switch to line mode.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	// Move to second changed line and unstage it.
	for range 4 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	unstaged, err := git.DiffPath(repo, "README.md", false, 1)
	if err != nil {
		t.Fatalf("unstaged diff: %v", err)
	}

	if !strings.Contains(staged, "+new-1") || strings.Contains(staged, "+new-2") || !strings.Contains(unstaged, "+new-2") {
		t.Fatalf("unexpected diffs after unstage line; status=%q\nSTAGED:\n%s\nUNSTAGED:\n%s", m.statusMsg, staged, unstaged)
	}
}

func TestLineModeStagesSingleLineInUntrackedFile(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "new.txt", "line-1\nline-2\nline-3\n")

	m := New(repo)
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := git.DiffPath(repo, "new.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+line-1") || !strings.Contains(staged, "+line-2") || strings.Contains(staged, "+line-3") {
		t.Fatalf("expected single line staged for untracked file; status=%q\nSTAGED:\n%s", m.statusMsg, staged)
	}
}

func TestLineModeUnstageOneOfAdjacentDeletedLinesDoesNotDuplicate(t *testing.T) {
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

	m := New(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := git.DiffPath(repo, "f.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated closing brace in staged diff:\n%s", staged)
	}
}

func TestLineModeUnstageBraceFromFirstHunkBlock(t *testing.T) {
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

	m := New(repo)
	m.ready = true

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updatedModel.(Model)

	// Move to the deletion of the closing brace line.
	for i := 0; i < 3; i++ {
		updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updatedModel.(Model)
	}
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updatedModel.(Model)

	staged, err := git.DiffPath(repo, "stage.go", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if strings.Contains(staged, "+    }") {
		t.Fatalf("unexpected duplicated brace in staged diff:\n%s", staged)
	}
}
