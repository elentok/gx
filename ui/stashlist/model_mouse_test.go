package stashlist

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/splitview"
)

// mustStashManyEntries pushes n single-file stashes so the list overflows a
// modest viewport height.
func mustStashManyEntries(t *testing.T, repo string, n int) {
	t.Helper()
	for i := range n {
		mustStashFile(t, repo, fmt.Sprintf("stash-%03d", i))
	}
}

// mustStashBigChange pushes a single stash whose diff and file tree are both
// large enough to overflow the detail pane's diff/filetree sub-regions.
func mustStashBigChange(t *testing.T, repo string) {
	t.Helper()
	before := make([]string, 0, 80)
	after := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		before = append(before, fmt.Sprintf("old-%03d", i))
		after = append(after, fmt.Sprintf("new-%03d", i))
	}
	// Named to sort first, so it's the file tree's initial selection and its
	// diff is what's on screen without needing extra file-tree navigation.
	testutil.WriteFile(t, repo, "aaa-scroll.txt", strings.Join(before, "\n")+"\n")
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "original\n")
	}
	testutil.CommitAll(t, repo, "base")

	testutil.WriteFile(t, repo, "aaa-scroll.txt", strings.Join(after, "\n")+"\n")
	for i := 1; i <= 40; i++ {
		testutil.WriteFile(t, repo, fmt.Sprintf("file%03d.txt", i), "changed\n")
	}
	cmd := exec.Command("git", "stash", "push", "-m", "big-change")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git stash push: %v\n%s", err, out)
	}
}

func TestWheelOverListScrollsListLeavesDetailUnchangedRegardlessOfFocus(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")
	mustStashManyEntries(t, repo, 30)

	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 20})

	// Pin keyboard focus to the detail pane so a successful list scroll
	// proves routing follows the cursor, not leftover keyboard focus.
	m.split = m.split.SetDetailFocused(true)

	listOffsetBefore := m.stashList.list.Offset()
	diffOffsetBefore := m.commitDetail.DiffScrollOffset()
	filetreeOffsetBefore := m.commitDetail.FileTreeScrollOffset()

	m = sendModel(m, tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})

	if m.stashList.list.Offset() <= listOffsetBefore {
		t.Fatalf("expected list to scroll on wheel over its rect, before=%d after=%d", listOffsetBefore, m.stashList.list.Offset())
	}
	if m.commitDetail.DiffScrollOffset() != diffOffsetBefore {
		t.Fatalf("expected diff scroll unchanged, before=%d after=%d", diffOffsetBefore, m.commitDetail.DiffScrollOffset())
	}
	if m.commitDetail.FileTreeScrollOffset() != filetreeOffsetBefore {
		t.Fatalf("expected filetree scroll unchanged, before=%d after=%d", filetreeOffsetBefore, m.commitDetail.FileTreeScrollOffset())
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected focus to remain on the detail pane")
	}
}

func TestWheelOverDiffScrollsDiffLeavesListAndFiletreeUnchanged(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashBigChange(t, repo)

	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	m.split = m.split.SetDetailFocused(false)

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible")
	}
	dw, dh := m.split.DetailSize()
	if dw < 90 {
		t.Fatalf("expected wide detail pane layout, got width=%d", dw)
	}
	_ = dh
	x, y := col+dw-1, row

	listOffsetBefore := m.stashList.list.Offset()
	diffOffsetBefore := m.commitDetail.DiffScrollOffset()
	filetreeOffsetBefore := m.commitDetail.FileTreeScrollOffset()

	m = sendModel(m, tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})

	if m.commitDetail.DiffScrollOffset() <= diffOffsetBefore {
		t.Fatalf("expected diff to scroll, before=%d after=%d", diffOffsetBefore, m.commitDetail.DiffScrollOffset())
	}
	if m.commitDetail.FileTreeScrollOffset() != filetreeOffsetBefore {
		t.Fatalf("expected filetree scroll unchanged, before=%d after=%d", filetreeOffsetBefore, m.commitDetail.FileTreeScrollOffset())
	}
	if m.stashList.list.Offset() != listOffsetBefore {
		t.Fatalf("expected list scroll unchanged, before=%d after=%d", listOffsetBefore, m.stashList.list.Offset())
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected focus to remain on the list")
	}
}

func TestWheelOverFiletreeScrollsFiletreeLeavesListAndDiffUnchanged(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	mustStashBigChange(t, repo)

	m := runModelInit(NewModel(repo, ui.Settings{}, keys.Manager{}))
	m = sendModel(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	m.split = m.split.SetDetailFocused(false)

	col, row, visible := m.split.DetailOrigin()
	if !visible {
		t.Fatal("expected detail pane visible")
	}
	x, y := col, row

	if got := m.split.HoverSideAt(x, y); got != splitview.HoverDetail {
		t.Fatalf("expected filetree coordinates to hover the detail pane, got %v", got)
	}

	listOffsetBefore := m.stashList.list.Offset()
	diffOffsetBefore := m.commitDetail.DiffScrollOffset()
	filetreeOffsetBefore := m.commitDetail.FileTreeScrollOffset()

	m = sendModel(m, tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})

	if m.commitDetail.FileTreeScrollOffset() <= filetreeOffsetBefore {
		t.Fatalf("expected filetree to scroll, before=%d after=%d", filetreeOffsetBefore, m.commitDetail.FileTreeScrollOffset())
	}
	if m.commitDetail.DiffScrollOffset() != diffOffsetBefore {
		t.Fatalf("expected diff scroll unchanged, before=%d after=%d", diffOffsetBefore, m.commitDetail.DiffScrollOffset())
	}
	if m.stashList.list.Offset() != listOffsetBefore {
		t.Fatalf("expected list scroll unchanged, before=%d after=%d", listOffsetBefore, m.stashList.list.Offset())
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected focus to remain on the list")
	}
}

func TestWheelOverNoOverflowRegionLeavesScrollStateUnchanged(t *testing.T) {
	t.Parallel()
	m := newReadyModel(t) // single small stash, single small file: nothing overflows

	listOffsetBefore := m.stashList.list.Offset()
	diffOffsetBefore := m.commitDetail.DiffScrollOffset()
	filetreeOffsetBefore := m.commitDetail.FileTreeScrollOffset()

	m = sendModel(m, tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelDown})

	if m.stashList.list.Offset() != listOffsetBefore {
		t.Fatalf("expected list offset unchanged, before=%d after=%d", listOffsetBefore, m.stashList.list.Offset())
	}
	if m.commitDetail.DiffScrollOffset() != diffOffsetBefore {
		t.Fatalf("expected diff offset unchanged, before=%d after=%d", diffOffsetBefore, m.commitDetail.DiffScrollOffset())
	}
	if m.commitDetail.FileTreeScrollOffset() != filetreeOffsetBefore {
		t.Fatalf("expected filetree offset unchanged, before=%d after=%d", filetreeOffsetBefore, m.commitDetail.FileTreeScrollOffset())
	}
}
