package log

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/splitview"
)

// --- Split view integration tests ---

func TestEnterOnCommitInCollapsedExpandsToSplit(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)

	// Row 1 is the first real commit (row 0 is pseudo-line).
	m.listPanel = m.listPanel.SetSelected(1)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed initially")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatalf("expected Split after Enter on commit, vis = collapsed=%v split=%v", m.split.IsCollapsed(), m.split.IsSplit())
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after expanding")
	}
}

func TestLOnCommitInCollapsedExpandsToSplit(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed initially")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected Split after l on commit")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l on commit")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l on commit")
	}
}

func TestLFromLogPanelFocusesOpenDetail(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsSplit() || !m.split.IsListFocused() {
		t.Fatal("expected open split with log focused")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected split to remain open after l")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after l from log panel")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after l from log panel")
	}
}

func TestEscFromDetailReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// Esc from detail → list focused, still split.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after Esc from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after Esc from detail")
	}
}

func TestQFromDetailReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after q from detail")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after q from detail")
	}
}

func TestHFromDetailFileTreeReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}
	if !m.commitDetail.IsFileTreeFocused() {
		t.Fatal("expected commit file tree focused after opening detail")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected h to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after h from detail file tree")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail file tree")
	}
}

func TestHFromDetailHeaderReturnsFocusToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused before pressing h")
	}
	if !m.commitDetail.IsHeaderFocused() {
		t.Fatal("expected commit header focused before pressing h")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected h to switch focus without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsSplit() {
		t.Fatal("expected still Split after h from detail header")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused after h from detail header")
	}
}

func TestEscFromDetailInternalFocusStepsBackWithoutMovingToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	// Open split view.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// Tab routes to commit model and cycles focus to header (or diff).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.commitDetail.HasInternalFocus() {
		t.Skip("commit detail has no internal focus after Tab — skipping")
	}

	// Esc should step back internally — NOT move split focus to list.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected esc to be handled internally without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail still focused after internal esc (should step back, not defocus detail)")
	}
}

func TestQFromDetailInternalFocusStepsBackWithoutMovingToList(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	m = updated.(Model)
	if !m.commitDetail.HasInternalFocus() {
		t.Skip("commit detail has no internal focus after Tab — skipping")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q to be handled internally without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail still focused after internal q")
	}
}

func TestEscFromListWhileSplitCollapses(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	// Esc → list focused.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	// Esc again → collapsed.
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed after second Esc")
	}
}

func TestQFromListWhileSplitCollapses(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected q from list split to collapse without cmd, got %T: %v", msg, msg)
	}
	if !m.split.IsCollapsed() {
		t.Fatal("expected Collapsed after q from list in split")
	}
}

func TestLogPaneFrameColorTracksSplitFocus(t *testing.T) {
	m := newTestModel()
	if m.listPanel.logPaneBorderColor(m.splitPane) != ui.ColorBorder {
		t.Fatal("expected collapsed log frame to use default border")
	}
	if m.listPanel.logPaneTitleColor(m.splitPane) != ui.ColorBlue {
		t.Fatal("expected collapsed log frame to use default title color")
	}

	m.split = splitview.NewSplit(m.listPanel, m.commitDetail)
	if m.listPanel.logPaneBorderColor(m.splitPane) != ui.ColorOrange {
		t.Fatal("expected log frame orange while split list is focused")
	}

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused")
	}
	if m.listPanel.logPaneBorderColor(m.splitPane) != ui.ColorBorder {
		t.Fatal("expected log frame inactive while detail is focused")
	}

	m.split, _ = m.split.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.listPanel.logPaneBorderColor(m.splitPane) != ui.ColorOrange {
		t.Fatal("expected log frame orange after focus returns to list")
	}
}

func TestFKeyTogglesFullscreenList(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.WithRows([]row{
		{kind: rowPseudoStatus},
		{kind: rowCommit, commit: git.LogEntry{FullHash: "abc", Subject: "c1"}},
	}).SetSelected(1)

	// f in collapsed state → fullscreen list.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)
	if !m.split.IsFullscreen() {
		t.Fatal("expected Fullscreen after f in collapsed")
	}
	if !m.split.IsListFocused() {
		t.Fatal("expected list focused in fullscreen")
	}
}

func TestFKeyOnDetailFocusedTogglesFullscreenDetail(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "commit a")

	m := newTestModelDefault(repo, "", settings)
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.listPanel = m.listPanel.SetSelected(1)

	// Expand to split (detail focused).
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused after Enter")
	}

	// f → fullscreen detail.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)
	if !m.split.IsFullscreen() {
		t.Fatal("expected Fullscreen after f with detail focused")
	}
	if !m.split.IsDetailFocused() {
		t.Fatal("expected detail focused in fullscreen")
	}
}

func TestToChordTogglesOrientation(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)

	// Press "t" then "o" to toggle orientation.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(Model)

	// At width 200, auto-orient is Vertical; "to" should override to Horizontal.
	if m.split.EffectiveOrientation() != splitview.Horizontal {
		t.Fatalf("expected Horizontal after 'to' at width 200, got %v", m.split.EffectiveOrientation())
	}
}

func TestTKeyWithSearchFocusedAppendsToQueryNotChord(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)
	m.search.Start("")

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)

	if m.search.Query() != "t" {
		t.Fatalf("expected search query %q, got %q", "t", m.search.Query())
	}
	if len(m.keys.Prefix()) != 0 {
		t.Fatalf("expected no pending chord prefix, got %v", m.keys.Prefix())
	}
}

func TestTChordStillWorksWhenSearchNotFocused(t *testing.T) {
	m := newTestModel()
	m.width = 200
	m.height = 40
	m.splitPane, m.listPanel, _ = m.syncSplitSize(m.width, m.height, m.listPanel)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = updated.(Model)

	if m.split.EffectiveOrientation() != splitview.Horizontal {
		t.Fatalf("expected Horizontal after 'to' at width 200, got %v", m.split.EffectiveOrientation())
	}
}
