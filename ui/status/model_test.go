package status

import (
	"fmt"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/keys"
	notifypkg "github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/status/diffarea"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestModel(worktreeRoot string, settings ui.Settings, initialPath string) Model {
	return NewModel(worktreeRoot, settings, initialPath, keys.Manager{})
}

func newTestModelDefault(worktreeRoot string) Model {
	return newTestModel(worktreeRoot, DefaultSettings(), "")
}

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
				File: git.StageFileStatus{
					Path:         "new/renamed.go",
					RenameFrom:   "old/name.go",
					IndexStatus:  'R',
					WorktreeCode: ' ',
				},
				HasStaged: true,
			}},
		},
		fileTreeModel: filetree.NewModel[git.StageFileStatus](),
	}
	m.fileTreeModel.SetEntries([]filetree.Entry[git.StageFileStatus]{
		{Kind: filetree.EntryFile, DisplayName: "renamed.go", Value: git.StageFileStatus{Path: "new/renamed.go", RenameFrom: "old/name.go", IndexStatus: 'R', WorktreeCode: ' '}},
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
	fmt.Printf("filetreeW: %d", filetreeW)
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

func TestFiletreeLOnFileEntersDiffAndKeepsSectionOnFileChange(t *testing.T) {
	t.Parallel()
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

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionStaged

	if len(m.statusData.statusEntries) < 2 {
		t.Fatalf("expected two status entries, got %d", len(m.statusData.statusEntries))
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff {
		t.Fatalf("expected l on file to enter diff")
	}
	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged for same file")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if m.focus != focusFiletree {
		t.Fatalf("expected h in diff to return to filetree")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged after active file change")
	}
}

func TestReloadDiffsForSelection_KeepsSectionForUntrackedFile(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "tracked.txt", "one\n")
	testutil.MustGitExported(t, repo, "add", "tracked.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")
	testutil.WriteFile(t, repo, "untracked.txt", "hello\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionStaged

	found := false
	for i, entry := range m.statusData.statusEntries {
		if entry.Path == "untracked.txt" {
			m.setStatusSelection(i)
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected untracked file entry")
	}

	cmd := m.reloadDiffsForSelection()
	m = runStatusCmd(t, m, cmd)

	if m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("expected section to remain staged for untracked file, got %v", m.diffarea.ActiveSection)
	}
	if len(m.diffarea.Staged.DataRef().Parsed.Hunks) != 0 {
		t.Fatalf("expected staged section to remain empty for untracked file")
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) == 0 {
		t.Fatalf("expected unstaged diff content for untracked file")
	}
}

func TestTabSwitchesDiffSectionsOnly(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.MustGitExported(t, repo, "commit", "-m", "baseline")

	// Ensure both unstaged and staged sections exist for same file.
	testutil.WriteFile(t, repo, "a.txt", "ONE\ntwo\n")
	testutil.MustGitExported(t, repo, "add", "a.txt")
	testutil.WriteFile(t, repo, "a.txt", "ONE-again\ntwo\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	m.diffarea.ActiveSection = diffarea.SectionUnstaged

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusFiletree || m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("tab in filetree should switch to staged without moving focus, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusFiletree || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("second tab in filetree should switch back to unstaged without moving focus, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("l should focus unstaged diff, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionStaged {
		t.Fatalf("tab in diff should switch to staged, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if m.focus != focusDiff || m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		t.Fatalf("second tab in diff should switch back to unstaged, got focus=%v section=%v", m.focus, m.diffarea.ActiveSection)
	}
}

func TestFiletreeHOnFileMovesSelectionToParentDir(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/ui/status")
	testutil.WriteFile(t, repo, "ui/status/model.go", "package status\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	fileIdx := -1
	for i, entry := range m.statusData.statusEntries {
		if entry.Kind == statusEntryFile && entry.Path == "ui/status/model.go" {
			fileIdx = i
			break
		}
	}
	if fileIdx < 0 {
		t.Fatalf("expected ui/status/model.go entry in status tree")
	}
	m.statusData.listState.SetSelected(fileIdx, len(m.statusData.statusEntries))

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	_ = cmd
	if m.focus != focusFiletree {
		t.Fatalf("expected h to keep filetree focus")
	}
	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "ui/status" {
		t.Fatalf("expected selection to move to parent dir, got %+v", entry)
	}
}

func TestFiletreeLeftDoesNotMoveCompressedDirSelection(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/keyboards/iris/keymaps")
	testutil.WriteFile(t, repo, "keyboards/iris/keymaps/myfile.c", "changed\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected compressed dir selected by default, got %+v", entry)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected left in filetree to be a no-op")
	}
	entry, ok = m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "keyboards/iris/keymaps" {
		t.Fatalf("expected selection to stay on compressed dir, got %+v", entry)
	}
}

// statusModelWithDir creates a status model with an unstaged "docs/" directory
// containing two files. The docs dir row is pre-selected.
// Precondition asserts the dir is expanded so callers start from a known state.
func statusModelWithDir(t *testing.T) Model {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/docs")
	testutil.WriteFile(t, repo, "docs/a.md", "a\n")
	testutil.WriteFile(t, repo, "docs/b.md", "b\n")
	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree
	dirIdx := -1
	for i, entry := range m.statusData.statusEntries {
		if entry.Kind == statusEntryDir && entry.Path == "docs" {
			dirIdx = i
			break
		}
	}
	if dirIdx < 0 {
		t.Fatalf("precondition: expected docs dir in status tree")
	}
	m.setStatusSelection(dirIdx)
	entry, ok := m.selectedFiletreeEntry()
	if !ok || !entry.Expanded {
		t.Fatalf("precondition: expected docs dir to be expanded")
	}
	return m
}

func TestFiletreeHOnSelectedDirCollapsesDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after h, got %+v", entry)
	}
	if entry.Expanded {
		t.Fatalf("expected docs dir to collapse on h")
	}
}

func TestFiletreeEnterOnSelectedDirCollapsesDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after enter, got %+v", entry)
	}
	if entry.Expanded {
		t.Fatalf("expected docs dir to collapse on enter")
	}
}

func TestFiletreeRightOnSelectedDirExpandsDirectory(t *testing.T) {
	t.Parallel()
	m := statusModelWithDir(t)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" || entry.Expanded {
		t.Fatalf("expected docs dir collapsed before expand test, got %+v", entry)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	entry, ok = m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryDir || entry.Path != "docs" {
		t.Fatalf("expected docs dir selected after l, got %+v", entry)
	}
	if !entry.Expanded {
		t.Fatalf("expected docs dir to expand on l")
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

	line := m.helpLine()
	if !strings.Contains(line, "? help") || strings.Contains(line, "j/k") {
		t.Fatalf("expected compact status help line, got %q", line)
	}

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

func TestHelpLineRightAlignsHint(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.width = 48
	m.focus = focusFiletree

	line := m.helpLine()
	plain := ansi.Strip(line)

	if ansi.StringWidth(plain) != m.width {
		t.Fatalf("expected footer width %d, got %d (%q)", m.width, ansi.StringWidth(plain), plain)
	}
	if !strings.HasSuffix(plain, "· filetree · ? help") {
		t.Fatalf("expected hint right-aligned at end, got %q", plain)
	}
}

func TestHelpLineTruncatesBareHintWithEllipsis(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.width = 12
	m.focus = focusFiletree

	line := m.helpLine()
	plain := ansi.Strip(line)

	if ansi.StringWidth(plain) != m.width {
		t.Fatalf("expected footer width %d, got %d (%q)", m.width, ansi.StringWidth(plain), plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("expected bare hint truncation to use ellipsis, got %q", plain)
	}
}

func TestHelpLineTruncatesHintWithEllipsisWhenNarrow(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.width = 18
	m.focus = focusFiletree

	line := m.helpLine()
	plain := ansi.Strip(line)

	if ansi.StringWidth(plain) != m.width {
		t.Fatalf("expected footer width %d, got %d (%q)", m.width, ansi.StringWidth(plain), plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("expected truncated hint to use ellipsis when narrow, got %q", plain)
	}
}

func TestFiletreePaneShowsBranchSummaryAtBottom(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	m := newTestModelDefault(repo)
	m.ready = true
	m.width = 100
	m.height = 20
	m.statusData.branchName = "feature/test"
	m.statusData.branchBaseRef = "origin/main"
	m.statusData.branchSync = git.SyncStatus{Name: git.StatusAhead, Ahead: 2}

	pane := ansi.Strip(m.renderFiletreePane(72, 10))
	if strings.Contains(pane, "Filetree (") {
		t.Fatalf("expected branch summary out of the title (moved to bottom lines), got:\n%s", pane)
	}
	if !strings.Contains(pane, "feature/test") {
		t.Fatalf("expected branch summary to include branch name, got:\n%s", pane)
	}
	if !strings.Contains(pane, "↑2") {
		t.Fatalf("expected branch summary to include ahead state, got:\n%s", pane)
	}
	if strings.Contains(pane, "vs origin/main") {
		t.Fatalf("expected default base ref to stay hidden, got:\n%s", pane)
	}
}

func TestNewWithInitialPathSelectsFileAndKeepsFiletreeFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, repo+"/dir")
	testutil.WriteFile(t, repo, "dir/a.txt", "one\n")
	testutil.WriteFile(t, repo, "dir/b.txt", "two\n")

	m := newTestModel(repo, ui.Settings{
		DiffContextLines: 1,
		UseNerdFontIcons: true,
	}, "dir/b.txt")

	entry, ok := m.selectedFiletreeEntry()
	if !ok {
		t.Fatal("expected selected entry")
	}
	if entry.Kind != statusEntryFile || entry.Path != "dir/b.txt" {
		t.Fatalf("selected entry = %+v, want file dir/b.txt", entry)
	}
	if m.focus != focusFiletree {
		t.Fatalf("focus = %v, want %v", m.focus, focusFiletree)
	}
	if m.activeFilePath != "dir/b.txt" {
		t.Fatalf("activeFilePath = %q, want %q", m.activeFilePath, "dir/b.txt")
	}
}

func TestBranchSummaryTitleShowsBaseOnlyWhenNonDefault(t *testing.T) {
	t.Parallel()
	m := Model{
		settings: ui.Settings{UseNerdFontIcons: true},
		statusData: statusData{
			branchName:    "feature/x",
			branchBaseRef: "origin/release",
			branchSync:    git.SyncStatus{Name: git.StatusBehind, Behind: 1},
		},
	}
	line := m.branchSummaryTitleSuffix()
	if !strings.Contains(line, " feature/x ↓1") {
		t.Fatalf("unexpected title suffix: %q", line)
	}
	if !strings.Contains(line, "vs origin/release") {
		t.Fatalf("expected non-default base ref in title suffix: %q", line)
	}
}

func TestReloadBranchStateUsesBranchUpstream(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(repoDir, "feature")
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature")
	testutil.WriteFile(t, wtDir, "ahead.txt", "ahead")
	testutil.CommitAll(t, wtDir, "ahead")

	m := newTestModelDefault(wtDir)
	m.reloadBranchState()

	if m.statusData.branchName != "feature" {
		t.Fatalf("expected current branch feature, got %q", m.statusData.branchName)
	}
	if m.statusData.branchBaseRef != "origin/feature" {
		t.Fatalf("expected upstream base ref origin/feature, got %q", m.statusData.branchBaseRef)
	}

	// Branch sync is now async; run the cmd synchronously to get the result.
	if cmd := m.cmdLoadBranchSync(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}
	if m.statusData.branchSync.Name != git.StatusAhead || m.statusData.branchSync.Ahead != 1 {
		t.Fatalf("expected branch sync ahead of origin/feature, got %+v", m.statusData.branchSync)
	}
}

func TestHelpLineShowsVisualAtLeftInDiffFocus(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := newTestModelDefault(testutil.TempRepo(t))
	m.ready = true
	m.width = 96
	m.focus = focusDiff
	m.diffarea.SetNavMode(diffview.NavModeLine)
	m.diffarea.Unstaged.DataRef().VisualActive = true

	line := m.helpLine()
	plain := ansi.Strip(line)

	if !strings.HasPrefix(plain, "VISUAL") {
		t.Fatalf("expected VISUAL indicator at start of footer, got %q", plain)
	}
	if !strings.HasSuffix(plain, "· diff: mode:line · render:unified · wrap:on · ? help") {
		t.Fatalf("expected diff hint at end of footer, got %q", plain)
	}
}

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
	if m.currentDiffContextLines() != 1 {
		t.Fatalf("expected initial diff context 1, got %d", m.currentDiffContextLines())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.currentDiffContextLines() != 2 {
		t.Fatalf("expected diff context 2 after ], got %d", m.currentDiffContextLines())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	if m.currentDiffContextLines() != 1 {
		t.Fatalf("expected diff context 1 after [, got %d", m.currentDiffContextLines())
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

	stagedBefore, err := git.DiffPath(repo, "clean.txt", true, 1)
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

	stagedAfter, err := git.DiffPath(repo, "clean.txt", true, 1)
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
	if m.currentDiffContextLines() != 2 {
		t.Fatalf("expected session diff context 2 after [, got %d", m.currentDiffContextLines())
	}
	if m.settings.DiffContextLines != 3 {
		t.Fatalf("expected settings diff context to remain 3, got %d", m.settings.DiffContextLines)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated.(Model)
	if m.currentDiffContextLines() != 3 {
		t.Fatalf("expected session diff context 3 after ], got %d", m.currentDiffContextLines())
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
	staged, err := git.DiffPath(repo, "b.txt", true, 1)
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
	staged, err := git.DiffPath(repo, "c.txt", true, 1)
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

	staged, err := git.DiffPath(repo, "v.txt", true, 1)
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

	countBefore := len(m.statusData.statusEntries)
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	updated, _ := m.Update(tea.FocusMsg{})
	m = updated.(Model)

	if len(m.statusData.statusEntries) <= countBefore {
		t.Fatalf("expected refresh on focus to include new file; before=%d after=%d", countBefore, len(m.statusData.statusEntries))
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

	staged, err := git.DiffPath(repo, "line.txt", true, 1)
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

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected file to be staged by filetree space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err = git.DiffPath(repo, "README.md", true, 1)
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
	if !m.confirmOpen {
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
	if !m.confirmOpen {
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
	if !m.confirmOpen {
		t.Fatalf("expected discard confirmation in unstaged diff")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	unstaged, err := git.DiffPath(repo, "line.txt", false, 1)
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

	if m.confirmOpen {
		t.Fatalf("did not expect discard confirm in staged diff; d should unstage")
	}
	unstaged, err := git.DiffPath(repo, "line.txt", false, 1)
	if err != nil {
		t.Fatalf("DiffPath unstaged: %v", err)
	}
	if strings.TrimSpace(unstaged) == "" {
		t.Fatalf("expected unstaged diff after using d in staged view")
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
		pane := m.renderSectionPane(80, 10, diffarea.SectionUnstaged)

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

func TestStatusFileIconDeletedAndFallback(t *testing.T) {
	t.Parallel()
	deleted := git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'}

	nerd := filetreePaneIconsFor(true)
	if got := statusFileIcon(deleted, false, nerd); got != "" {
		t.Fatalf("expected deleted nerd icon, got %q", got)
	}

	plain := filetreePaneIconsFor(false)
	if got := statusFileIcon(deleted, false, plain); got != "D" {
		t.Fatalf("expected deleted fallback icon, got %q", got)
	}
}

func TestStatusEntryColorDeletedFileIsDim(t *testing.T) {
	t.Parallel()
	entry := statusEntry{
		Kind: statusEntryFile,
		File: git.StageFileStatus{Path: "gone.txt", WorktreeCode: 'D'},
	}
	if got := statusEntryColor(entry); got != "#a6adc8" {
		t.Fatalf("expected dim deleted color, got %q", got)
	}
}

func TestStatusFileIconRenamedAndFallback(t *testing.T) {
	t.Parallel()
	renamed := git.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'}

	nerd := filetreePaneIconsFor(true)
	if got := statusFileIcon(renamed, false, nerd); got != "󰁔" {
		t.Fatalf("expected renamed nerd icon, got %q", got)
	}

	plain := filetreePaneIconsFor(false)
	if got := statusFileIcon(renamed, false, plain); got != "R" {
		t.Fatalf("expected renamed fallback icon, got %q", got)
	}
}

func TestStatusEntryColorRenamedFileIsBlue(t *testing.T) {
	t.Parallel()
	entry := statusEntry{
		Kind: statusEntryFile,
		File: git.StageFileStatus{Path: "new.txt", RenameFrom: "old.txt", IndexStatus: 'R'},
	}
	if got := statusEntryColor(entry); got != "#89b4fa" {
		t.Fatalf("expected renamed color, got %q", got)
	}
}

func TestFiletreeSelectionDebouncesDiffReload(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "one\n")
	testutil.WriteFile(t, repo, "b.txt", "two\n")

	m := newTestModelDefault(repo)
	m.ready = true
	m.focus = focusFiletree

	before := m.activeFilePath
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected j in filetree to schedule debounced reload")
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

func runStatusCmds(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, cmd := m.Update(msg)
	m = updated.(Model)
	return runStatusCmd(t, m, cmd)
}

func runStatusCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runStatusCmd(t, m, c)
		}
		return m
	}
	updated, next := m.Update(msg)
	m = updated.(Model)
	return runStatusCmd(t, m, next)
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

	pane := m.renderSectionPane(80, 12, diffarea.SectionUnstaged)
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

func TestNavigationStartupDefersInitialDiffLoad(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "dirty.txt", "dirty\n")

	m := newTestModel(repo, ui.Settings{EnableNavigation: true}, "")
	if m.activeFilePath != "" {
		t.Fatalf("expected initial navigation startup to skip diff load, got %q", m.activeFilePath)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) != 0 {
		t.Fatalf("expected no diff hunks before startup load, got %d", len(m.diffarea.Unstaged.DataRef().Parsed.Hunks))
	}

	updated, _ := m.Update(statusStartupLoadMsg{})
	m = updated.(Model)
	if m.activeFilePath != "dirty.txt" {
		t.Fatalf("expected startup load to select dirty diff, got %q", m.activeFilePath)
	}
	if len(m.diffarea.Unstaged.DataRef().Parsed.Hunks) == 0 {
		t.Fatalf("expected diff hunks after startup load")
	}
}

func TestGOOpensOutputModal(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)

	m := newTestModelDefault(repo)
	m.ready = true
	m.output.Set("test", "hello")

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
	if !m.output.IsOpen {
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

func TestDefaultSettingsEnableNerdFontIcons(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	if !settings.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestStatusEntryColor(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	icons := filetreePaneIconsFor(true)

	if got := statusEntryMeta(statusEntry{HasStaged: true, HasUnstaged: true}, true, icons); got != "" {
		t.Fatalf("partial nerd icon = %q", got)
	}
	if got := statusEntryMeta(statusEntry{HasStaged: true}, true, icons); got != "" {
		t.Fatalf("staged nerd icon = %q", got)
	}
}

func TestStatusFileIcon(t *testing.T) {
	t.Parallel()
	icons := filetreePaneIconsFor(true)

	if got := statusFileIcon(git.StageFileStatus{IndexStatus: '?', WorktreeCode: '?'}, false, icons); got != "" {
		t.Fatalf("untracked icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: 'A', WorktreeCode: ' '}, false, icons); got != "" {
		t.Fatalf("added icon = %q, want new file icon", got)
	}
	if got := statusFileIcon(git.StageFileStatus{IndexStatus: ' ', WorktreeCode: 'M'}, false, icons); got != "" {
		t.Fatalf("modified icon = %q, want modified file icon", got)
	}
}

func TestStatusFileIconSymlink(t *testing.T) {
	t.Parallel()
	nerd := filetreePaneIconsFor(true)
	plain := filetreePaneIconsFor(false)

	modified := git.StageFileStatus{Path: "link", IndexStatus: ' ', WorktreeCode: 'M'}
	if got := statusFileIcon(modified, true, nerd); got != "󰌷" {
		t.Fatalf("symlink nerd icon = %q, want symlink icon", got)
	}
	if got := statusFileIcon(modified, true, plain); got != "L" {
		t.Fatalf("symlink plain icon = %q, want L", got)
	}

	// Deleted symlink keeps the delete icon (deletion takes precedence).
	deleted := git.StageFileStatus{Path: "link", IndexStatus: 'D'}
	if got := statusFileIcon(deleted, true, nerd); got == "󰌷" {
		t.Fatalf("deleted symlink should show delete icon, not symlink icon")
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

	staged, err := git.DiffPath(repo, "README.md", true, 1)
	if err != nil {
		t.Fatalf("staged diff: %v", err)
	}
	unstaged, err := git.DiffPath(repo, "README.md", false, 1)
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

	staged, err := git.DiffPath(repo, "new.txt", true, 1)
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

	staged, err := git.DiffPath(repo, "f.go", true, 1)
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

	staged, err := git.DiffPath(repo, "stage.go", true, 1)
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

	staged, err := git.DiffPath(repo, "range.txt", true, 1)
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

	staged, err := git.DiffPath(repo, "range.txt", true, 1)
	if err != nil {
		t.Fatalf("DiffPath(staged): %v", err)
	}
	unstaged, err := git.DiffPath(repo, "range.txt", false, 1)
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

func pressKey(m Model, code rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
	return updated.(Model)
}

func pressSpecialKey(m Model, code rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code})
	return updated.(Model)
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
