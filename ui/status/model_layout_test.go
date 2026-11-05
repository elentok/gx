package status

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/tree"
	"strings"
	"testing"
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
