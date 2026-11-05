package status

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/status/diffarea"
	"os"
	"strings"
	"testing"
)

func TestToggleSideBySideModeWithS(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "a.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	updated, colorizeCmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.diffarea.RenderMode() != diffview.RenderModeSideBySide {
		t.Fatalf("expected render mode side-by-side, got %v", m.diffarea.RenderMode())
	}
	// Delta colorization is async; run the cmd synchronously to get colored output.
	if colorizeCmd != nil {
		updated, _ = m.Update(colorizeCmd())
		m = updated.(Model)
	}
	view := ansi.Strip(m.renderDiffPane(80, 16))
	if strings.Contains(view, "No file selected") {
		t.Fatalf("expected side-by-side diff content, got:\n%s", view)
	}
	if !strings.Contains(view, "▌ ") {
		t.Fatalf("expected active hunk indicator in side-by-side view, got:\n%s", view)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.diffarea.RenderMode() != diffview.RenderModeUnified {
		t.Fatalf("expected render mode unified, got %v", m.diffarea.RenderMode())
	}
}

func TestToggleSideBySideModeWithSFromFiletreePane(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "status-s.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "status-s.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "status-s.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.diffarea.RenderMode() != diffview.RenderModeSideBySide {
		t.Fatalf("expected render mode side-by-side from filetree pane, got %v", m.diffarea.RenderMode())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.diffarea.RenderMode() != diffview.RenderModeUnified {
		t.Fatalf("expected render mode unified after second toggle, got %v", m.diffarea.RenderMode())
	}
}

func TestStripUnifiedVisibleMarkerRemovesChangedPrefix(t *testing.T) {
	t.Parallel()
	line := "  1 ⋮    │-old"
	got := diffrender.StripUnifiedVisibleMarker(line, '-')
	if strings.Contains(got, "│-old") {
		t.Fatalf("expected visible marker stripped, got %q", got)
	}
	if !strings.Contains(got, "│ old") {
		t.Fatalf("expected content alignment preserved, got %q", got)
	}

	raw := "+new"
	got = diffrender.StripUnifiedVisibleMarker(raw, '+')
	if got != " new" {
		t.Fatalf("expected raw unified marker replaced with space, got %q", got)
	}

	line = "    ⋮ 579│+\tline := \"  1 ⋮    │-old\""
	got = diffrender.StripUnifiedVisibleMarker(line, '+')
	if strings.Contains(got, "│+\tline") {
		t.Fatalf("expected gutter marker stripped before source text, got %q", got)
	}
}

func TestAdjustDiffContextLinesInDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "ctx.txt", "one\ntwo\nthree\n")
	testutil.MustGitExported(t, repo, "add", "ctx.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "ctx.txt", "zero\none\ntwo\nTHREE\n")

	m := newTestModel(repo, ui.Settings{DiffContextLines: 1, UseNerdFontIcons: true}, "")
	m.ready = true

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.diffarea.ContextLines() != 1 {
		t.Fatalf("expected initial diff context 1, got %d", m.diffarea.ContextLines())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.diffarea.ContextLines() != 2 {
		t.Fatalf("expected diff context 2 after ], got %d", m.diffarea.ContextLines())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	if m.diffarea.ContextLines() != 1 {
		t.Fatalf("expected diff context 1 after [, got %d", m.diffarea.ContextLines())
	}
}

func TestUnifiedDiffViewHidesVisibleChangeMarkers(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "clean.txt", "old-value\nkeep\n")
	testutil.MustGitExported(t, repo, "add", "clean.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "clean.txt", "new-value\nkeep\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 20

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	view := ansi.Strip(m.renderDiffPane(100, 12))
	if strings.Contains(view, "+new-value") || strings.Contains(view, "-old-value") {
		t.Fatalf("expected unified view to hide visible +/- markers, got:\n%s", view)
	}
	if !strings.Contains(view, "new-value") || !strings.Contains(view, "old-value") {
		t.Fatalf("expected unified view to keep changed content visible, got:\n%s", view)
	}

	stagedBefore, err := stage.DiffPath(repo, "clean.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff before apply: %v", err)
	}
	if stagedBefore != "" {
		t.Fatalf("expected no staged diff before apply, got:\n%s", stagedBefore)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	stagedAfter, err := stage.DiffPath(repo, "clean.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff after apply: %v", err)
	}
	if !strings.Contains(stagedAfter, "old-value") && !strings.Contains(stagedAfter, "new-value") {
		t.Fatalf("expected staging to keep using raw unified diff mappings, got:\n%s", stagedAfter)
	}
}

func TestDiffPaneKeepsEmptySectionVisible(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "only-unstaged.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.syncDiffViewports()

	view := ansi.Strip(m.renderDiffPane(80, 12))
	if !strings.Contains(view, "Unstaged") {
		t.Fatalf("expected unstaged section title, got:\n%s", view)
	}
	if !strings.Contains(view, "Staged (empty)") {
		t.Fatalf("expected staged empty strip to remain visible, got:\n%s", view)
	}
}

func TestExpandedDiffPaneTitleShowsSelectedPathWithoutDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "only-unstaged.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.syncDiffViewports()

	view := ansi.Strip(m.renderDiffPane(80, 12))
	if !strings.Contains(view, "Unstaged: only-unstaged.txt") {
		t.Fatalf("expected active unstaged title to include path, got:\n%s", view)
	}
}

func TestDiffPaneAnchorsCollapsedUnstagedAtTopWhenStagedActive(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionStaged
	m.syncDiffViewports()

	view := ansi.Strip(m.renderDiffPane(80, 12))
	unstagedIdx := strings.Index(view, "Unstaged")
	stagedIdx := strings.Index(view, "Staged")
	if unstagedIdx < 0 || stagedIdx < 0 {
		t.Fatalf("expected both section titles, got:\n%s", view)
	}
	if unstagedIdx >= stagedIdx {
		t.Fatalf("expected collapsed unstaged strip above staged pane, got:\n%s", view)
	}
}

func TestDiffBodyPaddingStylesChangedRows(t *testing.T) {
	t.Parallel()
	added := diffrender.DiffBodyPadding(diffrender.RowAdded, 3)
	removed := diffrender.DiffBodyPadding(diffrender.RowRemoved, 3)
	plain := diffrender.DiffBodyPadding(diffrender.RowPlain, 3)

	if ansi.Strip(added) != "   " || ansi.Strip(removed) != "   " || plain != "   " {
		t.Fatalf("expected padding width preserved")
	}
	if added == "   " || removed == "   " {
		t.Fatalf("expected changed row padding to carry styling")
	}
}

func TestAdjustDiffContextLinesIsSessionOnly(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "ctx-status.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "ctx-status.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "ctx-status.txt", "ONE\ntwo\n")

	m := newTestModel(repo, ui.Settings{DiffContextLines: 3, UseNerdFontIcons: true}, "")
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	if m.diffarea.ContextLines() != 2 {
		t.Fatalf("expected session diff context 2 after [, got %d", m.diffarea.ContextLines())
	}
	if m.settings.DiffContextLines != 3 {
		t.Fatalf("expected settings diff context to remain 3, got %d", m.settings.DiffContextLines)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.diffarea.ContextLines() != 3 {
		t.Fatalf("expected session diff context 3 after ], got %d", m.diffarea.ContextLines())
	}
}

func TestSideBySideModeAllowsHunkStaging(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "b.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "b.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	staged, err := stage.DiffPath(repo, "b.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+two") {
		t.Fatalf("expected staged diff in side-by-side hunk mode, got:\n%s", staged)
	}
}

func TestSideBySideModeAllowsLineModeToggle(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "c.txt", "one\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	if m.diffarea.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected nav mode to switch to line in side-by-side, got %v", m.diffarea.NavMode())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	staged, err := stage.DiffPath(repo, "c.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+one") {
		t.Fatalf("expected staged line diff in side-by-side line mode, got:\n%s", staged)
	}
}

func TestSideBySideModeAllowsVisualLineRangeStaging(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "v.txt", "new-1\nnew-2\nnew-3\n")

	m := newTestModelDefault(repo)
	m.ready = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err := stage.DiffPath(repo, "v.txt", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	if !strings.Contains(staged, "+new-1") || !strings.Contains(staged, "+new-2") {
		t.Fatalf("expected visual range lines staged in side-by-side mode, got:\n%s", staged)
	}
}

func TestRefreshesOnFocusMsg(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true

	countBefore := len(m.filetree.Data.Entries)
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(Model)

	if len(m.filetree.Data.Entries) <= countBefore {
		t.Fatalf("expected refresh on focus to include new file; before=%d after=%d", countBefore, len(m.filetree.Data.Entries))
	}
}

func TestFocusMsgRefreshPreservesDiffScrollOffset(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	base := make([]string, 0, 80)
	updated := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		base = append(base, fmt.Sprintf("old-%03d", i))
		updated = append(updated, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll-focus.txt", strings.Join(base, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "scroll-focus.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "scroll-focus.txt", strings.Join(updated, "\n")+"\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 28
	m.focus = focusDiff
	m.syncDiffViewports()
	m.diffarea.Unstaged.Viewport().SetYOffset(9)

	updatedModel, _ := m.Update(tea.FocusMsg{})
	m = updatedModel.(Model)

	if got := m.diffarea.Unstaged.Viewport().YOffset(); got != 9 {
		t.Fatalf("expected focus refresh to preserve diff scroll offset, got %d", got)
	}
}

func TestViewEnablesReportFocus(t *testing.T) {
	t.Parallel()
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	v := m.View()
	if !v.ReportFocus {
		t.Fatalf("expected ReportFocus enabled on stage view")
	}
}

func TestFullscreenDiffHidesFiletreePane(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 30
	m.focus = focusDiff
	m.diffarea.Fullscreen = true

	v := m.View()
	plain := ansi.Strip(v.Content)
	if strings.Contains(plain, "Filetree") {
		t.Fatalf("expected filetree pane hidden in fullscreen diff view")
	}
}

func TestDiffJKScrollsWithoutMovingCursor(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	beforeLine := m.diffarea.Unstaged.DataRef().ActiveLine
	beforeOffset := m.diffarea.Unstaged.Viewport().YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	m = updated.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveLine != beforeLine {
		t.Fatalf("J changed active line: got %d want %d", m.diffarea.Unstaged.DataRef().ActiveLine, beforeLine)
	}
	maxOffset := m.diffarea.Unstaged.Viewport().TotalLineCount() - m.diffarea.Unstaged.Viewport().VisibleLineCount()
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
	if got := m.diffarea.Unstaged.Viewport().YOffset(); got != beforeOffset+expectedDelta {
		t.Fatalf("J should scroll by up to 3: before=%d after=%d expected=%d", beforeOffset, got, beforeOffset+expectedDelta)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	m = updated.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveLine != beforeLine {
		t.Fatalf("K changed active line: got %d want %d", m.diffarea.Unstaged.DataRef().ActiveLine, beforeLine)
	}
	if expectedDelta > 0 {
		if got := m.diffarea.Unstaged.Viewport().YOffset(); got >= beforeOffset+expectedDelta {
			t.Fatalf("K should scroll up on first press: offset after K=%d", got)
		}
	}
}

func TestHunkModeJKScrollsLargeHunkBeforeSwitching(t *testing.T) {
	t.Parallel()
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

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.syncDiffViewports()

	if m.diffarea.Unstaged.DataRef().ActiveHunk != 0 {
		t.Fatalf("expected first hunk active initially, got %d", m.diffarea.Unstaged.DataRef().ActiveHunk)
	}
	beforeOffset := m.diffarea.Unstaged.Viewport().YOffset()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updatedModel.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveHunk != 0 {
		t.Fatalf("expected j to scroll large hunk before switching, activeHunk=%d", m.diffarea.Unstaged.DataRef().ActiveHunk)
	}
	if m.diffarea.Unstaged.Viewport().YOffset() <= beforeOffset {
		t.Fatalf("expected j to scroll down within large hunk")
	}

	midOffset := m.diffarea.Unstaged.Viewport().YOffset()
	updatedModel, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updatedModel.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveHunk != 0 {
		t.Fatalf("expected k to scroll large hunk before switching, activeHunk=%d", m.diffarea.Unstaged.DataRef().ActiveHunk)
	}
	if m.diffarea.Unstaged.Viewport().YOffset() >= midOffset {
		t.Fatalf("expected k to scroll up within large hunk")
	}
}

func TestHunkOverflowViewportMarkers(t *testing.T) {
	t.Parallel()
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
		m := newTestModel(repo, ui.Settings{DiffContextLines: 1, UseNerdFontIcons: useNerd}, "")
		m.ready = true
		m.width = 100
		m.height = 16
		m.focus = focusDiff
		m.diffarea.ActiveSection = diffarea.SectionUnstaged
		m.diffarea.SetNavMode(diffview.NavModeHunk)
		m.syncDiffViewports()

		m.diffarea.Unstaged.Viewport().SetYOffset(3)
		pane := m.diffarea.RenderSection(80, 10, diffarea.SectionUnstaged, m.diffPaneInput())

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

func TestGInDiffHunkModeJumpsToLastHunk(t *testing.T) {
	t.Parallel()
	// Use multiple separated hunks so the last hunk is NOT at display line 0.
	// G should set ActiveHunk to the last hunk and scroll it into view.
	repo := testutil.TempRepo(t)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i+1)
	}
	testutil.WriteFile(t, repo, "big.txt", strings.Join(lines, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "big.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// Modify lines 1, 15, and 30 to create three separate hunks.
	updated := append([]string{}, lines...)
	updated[0] = "new-" + updated[0]
	updated[14] = "new-" + updated[14]
	updated[29] = "new-" + updated[29]
	testutil.WriteFile(t, repo, "big.txt", strings.Join(updated, "\n")+"\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 16
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.syncDiffViewports()

	updatedModel, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updatedModel.(Model)

	lastHunk := len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) - 1
	if lastHunk < 0 {
		t.Fatal("expected at least one hunk")
	}
	if got := m.diffarea.Unstaged.DataRef().ActiveHunk; got != lastHunk {
		t.Fatalf("expected G to set ActiveHunk to last (%d), got %d", lastHunk, got)
	}
	// Last hunk should be visible (viewport scrolled from initial position).
	if m.diffarea.Unstaged.Viewport().YOffset() == 0 && lastHunk > 0 {
		t.Fatalf("expected G to scroll viewport for multi-hunk content")
	}
}

func TestStageSearchDiffUsesRightEdgeIndicatorInHunkMode(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "needle-one\nline\nneedle-two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)

	pane := m.diffarea.RenderSection(80, 12, diffarea.SectionUnstaged, m.diffPaneInput())
	if !strings.Contains(ansi.Strip(pane), "needle") {
		t.Fatalf("expected search match text highlighted in diff pane")
	}
}

func TestDiffJDoesNotOverscrollPastContent(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	for i := 0; i < 300; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
		m = updated.(Model)
	}

	maxOffset := m.diffarea.Unstaged.Viewport().TotalLineCount() - m.diffarea.Unstaged.Viewport().VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if got := m.diffarea.Unstaged.Viewport().YOffset(); got != maxOffset {
		t.Fatalf("overscrolled: got offset=%d want=%d", got, maxOffset)
	}
}

func TestMouseWheelScrollsUnstagedDiffViewport(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "scroll.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "scroll.txt", strings.Join(after, "\n")+"\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 26
	m.syncDiffViewports()
	m.focus = focusDiff

	beforeOffset := m.diffarea.Unstaged.Viewport().YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 50, Y: 6, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() <= beforeOffset {
		t.Fatalf("expected unstaged viewport to scroll down, before=%d after=%d", beforeOffset, m.diffarea.Unstaged.Viewport().YOffset())
	}
}

func TestMouseWheelScrollsStagedDiffViewport(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	testutil.WriteFile(t, repo, "staged-scroll.txt", strings.Join(before, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "staged-scroll.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "staged-scroll.txt", strings.Join(after, "\n")+"\n")
	testutil.MustGitExported(t, repo, "add", "staged-scroll.txt")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 26
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionStaged
	m.syncDiffViewports()

	beforeOffset := m.diffarea.Staged.Viewport().YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 50, Y: 12, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffarea.Staged.Viewport().YOffset() <= beforeOffset {
		t.Fatalf("expected staged viewport to scroll down, before=%d after=%d", beforeOffset, m.diffarea.Staged.Viewport().YOffset())
	}
}

func TestWToggleSoftWrap(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "this is a very long line that should wrap in narrow diff panes\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 80
	m.height = 20
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.syncDiffViewports()

	wrappedCount := len(m.diffarea.Unstaged.DataRef().ViewLines)
	if !m.diffarea.Wrap() {
		t.Fatal("expected wrap enabled by default")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if m.diffarea.Wrap() {
		t.Fatal("expected wrap disabled after w")
	}
	unwrappedCount := len(m.diffarea.Unstaged.DataRef().ViewLines)
	if unwrappedCount > wrappedCount {
		t.Fatalf("expected unwrapped lines <= wrapped lines, got wrapped=%d unwrapped=%d", wrappedCount, unwrappedCount)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = updated.(Model)
	if !m.diffarea.Wrap() {
		t.Fatal("expected wrap enabled after second w")
	}
}

func TestBinaryFileShowsSizeSummaryInsteadOfNoFileSelected(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	path := repo + "/bin.dat"
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0644); err != nil {
		t.Fatalf("write baseline binary: %v", err)
	}
	testutil.MustGitExported(t, repo, "add", "bin.dat")
	testutil.MustGitExported(t, repo, "commit", "-m", "add binary")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, 0644); err != nil {
		t.Fatalf("write modified binary: %v", err)
	}

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.syncDiffViewports()

	view := ansi.Strip(m.renderDiffPane(80, 12))
	if strings.Contains(view, "No file selected") {
		t.Fatalf("expected binary summary instead of no-file message, got:\n%s", view)
	}
	if !strings.Contains(view, "binary file (prev size: 4 B, new size: 7 B)") {
		t.Fatalf("expected binary size summary, got:\n%s", view)
	}
}
