package status

import (
	"fmt"

	"github.com/elentok/gx/git"

	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/diffview"
	notifypkg "github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/tree"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUseStackedLayoutThreshold(t *testing.T) {
	t.Parallel()
	m := Model{width: 100}
	if !m.useStackedLayout() {
		t.Fatal("expected stacked layout at width 100")
	}
	m.width = 101
	if m.useStackedLayout() {
		t.Fatal("expected side-by-side layout at width 101")
	}
}

func TestSplitWidthUsesMinimumFiletreePaneWidthForShortContent(t *testing.T) {
	t.Parallel()
	m := Model{width: 160}
	filetreeW, diffW := m.splitWidth()
	if filetreeW != minFiletreePaneWidth {
		t.Fatalf("expected minimum filetree width %d, got %d", minFiletreePaneWidth, filetreeW)
	}
	if diffW != 160-minFiletreePaneWidth-1 {
		t.Fatalf("expected diff width %d, got %d", 160-minFiletreePaneWidth-1, diffW)
	}
}

func TestSplitWidthExpandsForLongVisibleFiletreeRows(t *testing.T) {
	t.Parallel()
	m := Model{
		width: 180,
		statusData: statusData{
			statusEntries: []statusEntry{{
				Kind:        statusEntryFile,
				DisplayName: "renamed.go",
				File: stage.StageFileStatus{
					Path:         "new/renamed.go",
					RenameFrom:   "old/name.go",
					IndexStatus:  'R',
					WorktreeCode: ' ',
				},
				HasStaged: true,
			}},
		},
		fileTreeModel: tree.NewModel[stage.StageFileStatus](),
	}
	m.fileTreeModel.SetEntries([]tree.Entry[stage.StageFileStatus]{
		{DisplayName: "renamed.go", Value: stage.StageFileStatus{Path: "new/renamed.go", RenameFrom: "old/name.go", IndexStatus: 'R', WorktreeCode: ' '}},
	})

	filetreeW, diffW := m.splitWidth()
	if filetreeW <= minFiletreePaneWidth {
		t.Fatalf("expected filetree pane to grow past minimum width, got %d", filetreeW)
	}
	if diffW >= 180-minFiletreePaneWidth {
		t.Fatalf("expected diff pane to shrink when filetree pane grows, got %d", diffW)
	}
	pane := ansi.Strip(m.renderFiletreePane(filetreeW, 10))
	if !strings.Contains(pane, "old/name.go -> new/renamed.go") {
		t.Fatalf("expected full renamed path to fit without truncation, got:\n%s", pane)
	}
}

func TestSplitWidthHonorsMaximumFiletreePaneWidth(t *testing.T) {
	t.Parallel()
	m := Model{
		width: 200,
		statusData: statusData{
			branchName:    "feature/some-extremely-verbose-branch-name-that-keeps-going",
			branchBaseRef: "origin/release/very-long-train-name",
			branchSync:    git.SyncStatus{Name: git.StatusDiverged, Ahead: 12, Behind: 8},
		},
	}

	filetreeW, diffW := m.splitWidth()
	if filetreeW != maxFiletreePaneWidth {
		t.Fatalf("expected filetree pane max width %d, got %d", maxFiletreePaneWidth, filetreeW)
	}
	if diffW != 200-maxFiletreePaneWidth-1 {
		t.Fatalf("expected diff width %d, got %d", 200-maxFiletreePaneWidth-1, diffW)
	}
}

func TestSplitWidthPreservesMinimumDiffWidth(t *testing.T) {
	t.Parallel()
	m := Model{
		width: 101,
		statusData: statusData{
			branchName:    "feature/some-extremely-verbose-branch-name-that-keeps-going",
			branchBaseRef: "origin/release/very-long-train-name",
			branchSync:    git.SyncStatus{Name: git.StatusDiverged, Ahead: 12, Behind: 8},
		},
	}

	filetreeW, diffW := m.splitWidth()
	if diffW != minDiffPaneWidth {
		t.Fatalf("expected minimum diff width %d, got %d", minDiffPaneWidth, diffW)
	}
	if filetreeW != 101-minDiffPaneWidth-1 {
		t.Fatalf("expected filetree width %d, got %d", 101-minDiffPaneWidth-1, filetreeW)
	}
}

func TestQAndEscFocusBehavior(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected nil cmd on esc")
	}
	m2 := updated.(Model)
	if m2.focus != focusFiletree {
		t.Fatalf("esc should move focus to filetree")
	}

	m2.focus = focusFiletree
	updated, cmd = m2.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q in filetree should quit")
	}
}

func TestQAlwaysQuitsFromDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("expected q to quit from diff focus")
	}
}

func TestHelpOverlayToggleAndCompactStatusBar(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 40
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(Model)
	if !m.help.IsOpen {
		t.Fatalf("expected help overlay to open")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if !m.help.IsOpen {
		t.Fatalf("expected help overlay to stay open while scrolling")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.help.IsOpen {
		t.Fatalf("expected help overlay to close on esc")
	}
}

func TestYankFilenameWithYFInFiletreeView(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "a.txt" {
		t.Fatalf("expected yanked filename, got %q", got)
	}
}

func TestYankLocationWithYLInFiletreeViewYanksFilenameOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "b.txt", "one\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if got != "@b.txt" {
		t.Fatalf("expected yl in filetree to yank filename only, got %q", got)
	}
}

func TestYankAllContextWithYAInDiffLineModeIncludesLocationAndSelectedLine(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "c.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "c.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "c.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	if !strings.Contains(got, "@c.txt L") {
		t.Fatalf("expected ya output to include file and line, got %q", got)
	}
	if !strings.Contains(got, "-old-2") {
		t.Fatalf("expected ya output to include selected line content, got %q", got)
	}
	if strings.Contains(got, "-old-1") || strings.Contains(got, "+new") {
		t.Fatalf("expected ya output to exclude non-selected line content, got %q", got)
	}
}

func TestYankFilenameWithYFInDiffLineModeYanksFilenameOnly(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "d.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "d.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "d.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if got != "d.txt" {
		t.Fatalf("expected yf output to be filename only, got %q", got)
	}
}

func TestYankLocationOnlyWithYLInDiffVisualModeYanksOnlyLocation(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "e.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "e.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "e.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)

	if !strings.HasPrefix(got, "@e.txt L") {
		t.Fatalf("expected yl output to include only file and line range, got %q", got)
	}
	if strings.Contains(got, "\n") || strings.Contains(got, "-old") || strings.Contains(got, "+new") {
		t.Fatalf("expected yl output to exclude content lines, got %q", got)
	}
}

func TestYankSelectionOnlyWithYYInDiffLineMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "f.txt", "old-1\nold-2\n")
	testutil.MustGitExported(t, repo, "add", "f.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "f.txt", "new-1\nnew-2\n")

	var got string
	prev := stageClipboardWrite
	stageClipboardWrite = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { stageClipboardWrite = prev })

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if strings.Contains(got, "@f.txt") {
		t.Fatalf("expected yy output to be raw selection only, got %q", got)
	}
	if got != "-old-1" {
		t.Fatalf("expected yy output to yank only active line, got %q", got)
	}
}

func TestGInFiletreeAndDiffJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")
	testutil.WriteFile(t, repo, "c.txt", "three\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.statusData.listState.SetSelected(0, len(m.statusData.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.statusData.listState.Selected() != len(m.statusData.statusEntries)-1 {
		t.Fatalf("expected G to jump filetree selection to bottom, got %d", m.statusData.listState.Selected())
	}

	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)
	if len(m.diffarea.Unstaged.DataRef().Parsed.Changed) == 0 {
		t.Fatalf("expected unstaged changes in diff view")
	}
	m.diffarea.Unstaged.DataRef().ActiveLine = 0

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.diffarea.Unstaged.DataRef().ActiveLine != len(m.diffarea.Unstaged.DataRef().Parsed.Changed)-1 {
		t.Fatalf("expected G to jump active diff line to bottom, got %d", m.diffarea.Unstaged.DataRef().ActiveLine)
	}
}

func TestUppercaseGUsingShiftedCodeJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.statusData.listState.SetSelected(0, len(m.statusData.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "G", ShiftedCode: 'G'})
	m = updated.(Model)
	if m.statusData.listState.Selected() != len(m.statusData.statusEntries)-1 {
		t.Fatalf("expected shifted G to jump to bottom, got %d", m.statusData.listState.Selected())
	}
}

func TestUppercaseGUsingShiftModifierJumpsBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.statusData.listState.SetSelected(0, len(m.statusData.statusEntries))

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g", Mod: tea.ModShift})
	m = updated.(Model)
	if m.statusData.listState.Selected() != len(m.statusData.statusEntries)-1 {
		t.Fatalf("expected shifted modifier G to jump to bottom, got %d", m.statusData.listState.Selected())
	}
}

func TestCtrlDAndCtrlUScrollFiletreeAndDiff(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	for i := 0; i < 16; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("f%02d.txt", i), "x\n")
	}

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 120
	m.height = 24
	m.focus = focusFiletree
	beforeSel := m.statusData.listState.Selected()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.statusData.listState.Selected() <= beforeSel {
		t.Fatalf("expected ctrl+d to move filetree selection down")
	}

	midSel := m.statusData.listState.Selected()
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.statusData.listState.Selected() >= midSel {
		t.Fatalf("expected ctrl+u to move filetree selection up")
	}

	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeHunk)
	m.syncDiffViewports()
	beforeOffset := m.diffarea.Unstaged.Viewport().YOffset()

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() < beforeOffset {
		t.Fatalf("expected ctrl+d to scroll diff down")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() > beforeOffset {
		t.Fatalf("expected ctrl+u to scroll diff up")
	}
}

func TestStageSearchFiletreeModeAndNavigation(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")
	testutil.WriteFile(t, repo, "apricot.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.fileTreeModel.Search().Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after /")
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.fileTreeModel.Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple filetree search matches, got %d", m.fileTreeModel.Search().MatchesCount())
	}

	first := m.statusData.listState.Selected()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.fileTreeModel.Search().Mode() != search.SearchModeResults || !m.fileTreeModel.Search().HasQuery() || m.fileTreeModel.Search().MatchesCount() == 0 {
		t.Fatalf("expected enter to show search results mode while keeping highlights")
	}
	if pane := ansi.Strip(m.renderFiletreePane(40, 10)); !strings.Contains(pane, "1/2") {
		t.Fatalf("expected persistent search counter in filetree frame, got %q", pane)
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.statusData.listState.Selected() == first {
		t.Fatalf("expected n to move to next search result")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.fileTreeModel.Search().Mode() != search.SearchModeNone || m.fileTreeModel.Search().HasQuery() || m.fileTreeModel.Search().MatchesCount() != 0 {
		t.Fatalf("expected esc to clear active search state")
	}
}

func TestFiletreeSearchModeShowsSearchBox(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "apple.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 60
	m.height = 20
	m.focus = focusFiletree

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)

	lines := m.visibleStatusLines(30, 20)
	combined := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(combined, "Search") {
		t.Fatalf("expected rendered lines to contain 'Search', got %q", combined)
	}
}

func TestStageSearchDiffModeAndPrevNextKeys(t *testing.T) {
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	if m.diffarea.ActiveSectionModel().Search().Mode() != search.SearchModeInput {
		t.Fatalf("expected diff search input mode after /")
	}

	for _, r := range []rune{'n', 'e', 'e', 'd', 'l', 'e'} {
		m = runStatusCmds(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.diffarea.ActiveSectionModel().Search().MatchesCount() < 2 {
		t.Fatalf("expected multiple diff search matches, got %d", m.diffarea.ActiveSectionModel().Search().MatchesCount())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.diffarea.ActiveSectionModel().Search().Mode() != search.SearchModeResults || !m.diffarea.ActiveSectionModel().Search().HasQuery() || m.diffarea.ActiveSectionModel().Search().MatchesCount() == 0 {
		t.Fatalf("expected enter to show search results mode while keeping highlights")
	}
	if m.diffarea.NavMode() != diffview.NavModeLine {
		t.Fatalf("expected enter after diff search to switch to line mode")
	}
	first := m.diffarea.ActiveSectionModel().Search().Cursor()
	firstLine := m.diffarea.Unstaged.DataRef().ActiveLine

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.diffarea.ActiveSectionModel().Search().Cursor() == first {
		t.Fatalf("expected n to move to next diff result")
	}
	if m.diffarea.Unstaged.DataRef().ActiveLine == firstLine {
		t.Fatalf("expected n to move active diff line to next match")
	}

	m = runStatusCmds(t, m, tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N'})
	if m.diffarea.ActiveSectionModel().Search().Cursor() != first {
		t.Fatalf("expected N to move back to previous diff result")
	}

	// Moving cursor to a matched line should update the search counter cursor.
	startCursor := m.diffarea.ActiveSectionModel().Search().Cursor()
	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
		if m.diffarea.ActiveSectionModel().Search().Cursor() != startCursor {
			break
		}
	}
	if m.diffarea.ActiveSectionModel().Search().Cursor() == startCursor {
		t.Fatalf("expected diff cursor movement to sync search cursor when reaching a match")
	}
}

func TestSearchConfirmedSyncsToCrossPane(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	// Stage a change so the staged diff section has content containing "needle".
	testutil.WriteFile(t, repo, "README.md", "needle-one\nneedle-two\n")
	testutil.MustGitExported(t, repo, "add", "README.md")
	// Also leave an unstaged modification so both sections have diffs.
	testutil.WriteFile(t, repo, "README.md", "needle-one\nneedle-two\nneedle-three\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	// Activate search and type a query
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)
	for _, r := range []rune{'n', 'e', 'e', 'd', 'l', 'e'} {
		m = runStatusCmds(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Confirm with Enter
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	// Active (unstaged) section should be in results mode
	active := m.diffarea.ActiveSectionModel().Search()
	if active.Mode() != search.SearchModeResults {
		t.Fatalf("active section: expected SearchModeResults after Enter, got %v", active.Mode())
	}
	if !active.HasQuery() {
		t.Fatal("active section: expected HasQuery=true after Enter")
	}

	// Inactive (staged) section should have the query synced and be in results mode
	inactive := m.diffarea.InactiveSectionModel().Search()
	if !inactive.HasQuery() {
		t.Fatal("inactive section: expected HasQuery=true after cross-pane sync")
	}
	if inactive.Mode() != search.SearchModeResults {
		t.Fatalf("inactive section: expected SearchModeResults, got %v", inactive.Mode())
	}

	// Counter for inactive section should be non-empty (advisor check)
	counter := m.searchCounterForDiffSection(diffarea.SectionStaged)
	if inactive.MatchesCount() > 0 && counter == "" {
		t.Fatal("expected non-empty counter for inactive section when it has matches")
	}
}

func TestCCTriggersCommitCommand(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "c" {
		t.Fatalf("expected chord prefix=c after first c, got %v", m.keys.Prefix())
	}
	// Chord hints are shown in the chord overlay via Manager.ChordHints().
	hints := m.keys.ChordHints()
	if len(hints) == 0 {
		t.Fatalf("expected ChordHints to return bindings for c prefix, got none")
	}
	found := false
	for _, h := range hints {
		if strings.Contains(h.Desc, "commit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected \"commit\" in ChordHints descriptions")
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatalf("second c should launch commit command")
	}
	m = updated.(Model)
	if m.keys.Prefix() != nil {
		t.Fatalf("expected prefix cleared after cc, got %v", m.keys.Prefix())
	}
}

func TestCMInFiletreeDoesNothing(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatalf("first c should not launch command")
	}
	m = updated.(Model)

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd != nil {
		t.Fatalf("cm in filetree should not launch command")
	}
	m = updated.(Model)
}

func TestAskAIInDiffWithoutFileContextShowsError(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("aa (Ask AI) without file context should return a warning notification cmd")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notifypkg.NotifyMsg)
	if !ok {
		t.Fatalf("expected NotifyMsg, got %T", msg)
	}
	if notifyMsg.Message != "no file context for comment" {
		t.Fatalf("notification = %q, want %q", notifyMsg.Message, "no file context for comment")
	}
}

func TestYShowsBindingDrivenYankHint(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil {
		t.Fatalf("first y should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "y" {
		t.Fatalf("expected chord prefix=y, got %v", m.keys.Prefix())
	}

	// Chord hints are shown in the chord overlay via Manager.ChordHints().
	hints := m.keys.ChordHints()
	allDescs := ""
	for _, h := range hints {
		allDescs += " " + h.Key + " " + h.Desc
	}
	// yank-for-AI moved off the 'y' chord to 'ay'; the legacy 'ya' alias is
	// hidden, so it no longer appears in the 'y' chord hint overlay.
	for _, want := range []string{"y", "content", "l", "location", "f", "filename"} {
		if !strings.Contains(allDescs, want) {
			t.Fatalf("expected yank hint %q in ChordHints descriptions %q", want, allDescs)
		}
	}
}

func TestGGJumpsToTop(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.statusData.statusEntries = []statusEntry{{Kind: statusEntryFile}, {Kind: statusEntryFile}, {Kind: statusEntryFile}}
	m.statusData.listState.SetSelected(2, len(m.statusData.statusEntries))

	// First g sets chord prefix
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "g" {
		t.Fatalf("expected chord prefix=g after first g, got %v", m.keys.Prefix())
	}

	// Second g jumps to top
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd == nil {
		t.Fatalf("gg should schedule a diff reload after jumping to top")
	}
	m = updated.(Model)
	if m.statusData.listState.Selected() != 0 {
		t.Fatalf("expected gg to jump to top, got selected=%d", m.statusData.listState.Selected())
	}
}

func TestLTriggersLazygitLogCommand(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'L', Text: "L", ShiftedCode: 'L', Mod: tea.ModShift})
	if cmd == nil {
		t.Fatalf("L should launch lazygit log command")
	}
}

func TestGOOpensOutputModal(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.git.SetOutput("test", "hello")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Fatalf("first g should not launch command")
	}
	m = updated.(Model)
	if len(m.keys.Prefix()) == 0 || m.keys.Prefix()[0] != "g" {
		t.Fatalf("expected chord prefix=g after first g, got %v", m.keys.Prefix())
	}

	// Opening the output modal is itself a disrupting event for the image-diff
	// overlay (ADR 0010: a stale placement would otherwise occlude the modal),
	// so cmd is expected to carry that internal housekeeping — not an external
	// command launch, which is what this test guards against.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(Model)
	if !m.git.OutputOpen() {
		t.Fatalf("expected go to open output modal")
	}
}

func TestEOpensEditorFromFiletreeAndDiff(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	// ee chord: filetree focus
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in filetree view to launch editor command")
	}

	// ee chord: diff focus
	m.focus = focusDiff
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected ee in diff view to launch editor command")
	}
}

func TestEditChordSplitVariants(t *testing.T) {
	t.Setenv("EDITOR", "true")
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "edit.txt", "one\n")

	chords := []struct {
		name   string
		second string
	}{
		{"es (split)", "s"},
		{"ev (vsplit)", "v"},
		{"et (tab)", "t"},
	}

	for _, tt := range chords {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelDefault(repo)
			m.ready = true
			m.focus = focusFiletree

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
			m = updated.(Model)
			updated, cmd := m.Update(tea.KeyPressMsg{Text: tt.second})
			_ = updated.(Model)
			if cmd == nil {
				t.Fatalf("expected e%s chord to return a non-nil cmd", tt.second)
			}
		})
	}
}

func TestEditorLineForCurrentSelectionInDiffMode(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "line.txt", "old-1\nold-2\nold-3\n")
	testutil.MustGitExported(t, repo, "add", "line.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "line.txt", "new-1\nnew-2\nnew-3\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusDiff
	m.diffarea.ActiveSection = diffarea.SectionUnstaged
	m.diffarea.SetNavMode(diffview.NavModeLine)

	for i, cl := range m.diffarea.Unstaged.DataRef().Parsed.Changed {
		if cl.NewLine == 2 {
			m.diffarea.Unstaged.DataRef().ActiveLine = i
			break
		}
	}

	line := m.editorLineForCurrentSelection()
	if line != 2 {
		t.Fatalf("editorLineForCurrentSelection()=%d, want 2", line)
	}
}

func TestMouseWheelWhileHelpOpenScrollsHelpNotDiff(t *testing.T) {
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
	m.width = 70
	m.height = 20
	m.syncDiffViewports()
	m.focus = focusDiff
	m.help.Open(m.width, m.height)

	beforeDiffOffset := m.diffarea.Unstaged.Viewport().YOffset()
	beforeHelpOffset := m.help.Viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 30, Y: 6, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffarea.Unstaged.Viewport().YOffset() != beforeDiffOffset {
		t.Fatalf("expected unstaged viewport not to scroll while help is open, before=%d after=%d", beforeDiffOffset, m.diffarea.Unstaged.Viewport().YOffset())
	}
	if m.help.Viewport.YOffset() <= beforeHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", beforeHelpOffset, m.help.Viewport.YOffset())
	}
}

// TestGChordBindingsReachableFromDiffFocus verifies that g+o, g+p, g+h, and
// g+esc are all reachable when the diff panel is focused. Previously the first
// 'g' was consumed by diffview's g+g chord prefix, leaving status chords
// unreachable.
func TestGChordBindingsReachableFromDiffFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "changed\n")

	newDiffFocusModel := func(t *testing.T) Model {
		t.Helper()
		m := newTestModelDefault(repo)
		m.ready = true
		m.focus = focusDiff
		return m
	}

	t.Run("g+esc cancels chord without side effects", func(t *testing.T) {
		m := newDiffFocusModel(t)
		m = pressKey(m, 'g')
		if len(m.keys.Prefix()) != 1 {
			t.Fatalf("after first g: expected status prefix len 1, got %d", len(m.keys.Prefix()))
		}
		m = pressSpecialKey(m, tea.KeyEsc)
		if len(m.keys.Prefix()) != 0 {
			t.Fatalf("after g+esc: expected status prefix cleared, got len %d", len(m.keys.Prefix()))
		}
		if m.focus != focusDiff {
			t.Fatalf("after g+esc: expected to remain in diff focus, got %v", m.focus)
		}
	})

	t.Run("g+g still scrolls diff to top via diffview", func(t *testing.T) {
		m := newDiffFocusModel(t)
		m = pressKey(m, 'g')
		if !m.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after first g: expected diffview pending chord=true")
		}
		m = pressKey(m, 'g')
		// diffview should have won the chord; pending chord cleared.
		if m.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after g+g: expected diffview pending chord=false, diffview handled the chord")
		}
	})

	t.Run("subsequent j navigation works after g+o chord", func(t *testing.T) {
		// Prime with diff content so j has something to navigate
		testutil.MustGitExported(t, repo, "add", "README.md")
		testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
		testutil.WriteFile(t, repo, "README.md", "line1\nline2\nline3\nchanged\n")
		m2 := newTestModelDefault(repo)
		m2.ready = true
		m2.width = 100
		m2.height = 20
		m2.focus = focusDiff
		m2 = runStatusCmd(t, m2, m2.reloadDiffsForSelection())
		// Press g+o (which opens output panel if there's output, or shows "no output")
		m2 = pressKey(m2, 'g')
		m2 = pressKey(m2, 'o')
		// g prefix in status should be cleared
		if len(m2.keys.Prefix()) != 0 {
			t.Fatalf("after g+o: expected status prefix cleared, got %v", m2.keys.Prefix())
		}
		// diffview pending chord should be cleared.
		if m2.diffarea.ActiveSectionModel().HasPendingChord() {
			t.Fatalf("after g+o: expected diffview pending chord=false")
		}
	})
}

func TestModalOpenAfterPressingPull(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	m := newTestModelDefault(repoDir)

	// Press 'p' to open pull.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated.(Model)

	if !m.ModalOpen() {
		t.Fatal("expected ModalOpen()=true immediately after pressing pull key")
	}
}
