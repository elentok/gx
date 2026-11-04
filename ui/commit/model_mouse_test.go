package commit

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/elentok/gx/testutil"
	"strings"
	"testing"
)

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

func TestMouseWheelWhileHelpOpenScrollsHelpNotDiff(t *testing.T) {
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
	m.help.Open(m.width, m.height)

	bodyH, contentH := m.layoutHeights()
	diffX := m.filesPaneWidth(contentH) + 1
	diffY := bodyH + 1

	beforeDiffOffset := m.diffModel.Viewport().YOffset()
	beforeHelpOffset := m.help.Viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: diffX, Y: diffY, Button: tea.MouseWheelDown})
	m = updated.(Model)
	if m.diffModel.Viewport().YOffset() != beforeDiffOffset {
		t.Fatalf("expected diff viewport not to scroll while help is open, before=%d after=%d", beforeDiffOffset, m.diffModel.Viewport().YOffset())
	}
	if m.help.Viewport.YOffset() <= beforeHelpOffset {
		t.Fatalf("expected help viewport to scroll on wheel while open, before=%d after=%d", beforeHelpOffset, m.help.Viewport.YOffset())
	}
}
