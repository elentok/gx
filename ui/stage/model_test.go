package stage

import (
	"testing"

	"gx/git"
	"gx/testutil"

	tea "charm.land/bubbletea/v2"
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

func TestSpaceStagesSingleLineInLineMode(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "README.md", "# test\nadded\n")

	m := New(repo)
	m.ready = true
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	m.applySelection()

	staged, err := git.DiffPath(repo, "README.md", true)
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

	staged, err := git.DiffPath(repo, "README.md", true)
	if err != nil {
		t.Fatalf("DiffPath cached: %v", err)
	}
	if staged == "" {
		t.Fatalf("expected file to be staged by status space")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)

	staged, err = git.DiffPath(repo, "README.md", true)
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
	m.focus = focusDiff
	m.section = sectionUnstaged
	m.navMode = navLine

	beforeLine := m.unstaged.activeLine
	beforeScroll := m.unstaged.scroll

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("J changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
	if m.unstaged.scroll <= beforeScroll {
		t.Fatalf("J did not scroll: before=%d after=%d", beforeScroll, m.unstaged.scroll)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	m = updated.(Model)
	if m.unstaged.activeLine != beforeLine {
		t.Fatalf("K changed active line: got %d want %d", m.unstaged.activeLine, beforeLine)
	}
}
