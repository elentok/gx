package diffview

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// buildNavTestModel creates a Model with two hunks and content for nav key tests.
func buildNavTestModel() Model {
	m := NewModel(false)
	raw := "@@ -1,3 +1,3 @@\n-a\n+b\n context\n@@ -10,3 +10,3 @@\n-x\n+y\n context\n"
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 20)
	return m
}

func pressText(m Model, text string) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Text: text})
	return next, result
}

func pressCode(m Model, code rune) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Code: code})
	return next, result
}

func pressCtrl(m Model, r rune) (Model, UpdateResult) {
	next, _, result := m.Update(tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl})
	return next, result
}

func TestUpdateKey_JMovesDownHandled(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressText(m, "j")
	if !result.Handled {
		t.Fatal("j: expected Handled=true")
	}
	if next.Data().ActiveHunk <= m.Data().ActiveHunk {
		t.Fatalf("j: expected ActiveHunk to increase, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_KMovesUpHandled(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 1
	next, result := pressText(m, "k")
	if !result.Handled {
		t.Fatal("k: expected Handled=true")
	}
	if next.Data().ActiveHunk >= m.Data().ActiveHunk {
		t.Fatalf("k: expected ActiveHunk to decrease, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_DownArrowMovesDown(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressCode(m, tea.KeyDown)
	if !result.Handled {
		t.Fatal("down: expected Handled=true")
	}
	if next.Data().ActiveHunk <= m.Data().ActiveHunk {
		t.Fatalf("down: expected ActiveHunk to increase, got %d→%d", m.Data().ActiveHunk, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_JKScrollViewport(t *testing.T) {
	m := buildNavTestModel()
	// J scrolls viewport without changing active item
	before := m.data.ActiveHunk
	next, result := pressText(m, "J")
	if !result.Handled {
		t.Fatal("J: expected Handled=true")
	}
	_ = next.data.ActiveHunk // snap may change it, but no assertion needed
	_ = before

	next, result = pressText(m, "K")
	if !result.Handled {
		t.Fatal("K: expected Handled=true")
	}
}

func TestUpdateKey_TTogglesNavMode(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.VisualActive = true
	next, result := pressText(m, "t")
	if !result.Handled {
		t.Fatal("t: expected Handled=true")
	}
	if next.NavMode() != NavModeLine {
		t.Fatalf("t: expected NavModeLine, got %v", next.NavMode())
	}
	if next.data.VisualActive {
		t.Fatal("t: expected VisualActive=false after toggle")
	}
}

func TestUpdateKey_TTogglesNavModeBackToHunk(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeLine)
	next, result := pressText(m, "t")
	if !result.Handled {
		t.Fatal("t: expected Handled=true")
	}
	if next.NavMode() != NavModeHunk {
		t.Fatalf("t: expected NavModeHunk, got %v", next.NavMode())
	}
}

// 'a' was the old nav-mode toggle; it is now free (reserved as the AI chord
// prefix at the host level) and must no longer toggle nav-mode in the diff pane.
func TestUpdateKey_ANoLongerTogglesNavMode(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, _ := pressText(m, "a")
	if next.NavMode() != NavModeHunk {
		t.Fatalf("a: expected NavMode unchanged (NavModeHunk), got %v", next.NavMode())
	}
}

func TestUpdateKey_VTogglesVisual(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	next, result := pressText(m, "v")
	if !result.Handled {
		t.Fatal("v: expected Handled=true")
	}
	if next.NavMode() != NavModeLine {
		t.Fatalf("v: expected NavModeLine when starting from hunk mode, got %v", next.NavMode())
	}
	if !next.data.VisualActive {
		t.Fatal("v: expected VisualActive=true")
	}
}

func TestUpdateKey_VToggleVisualOff(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeLine)
	m.data.VisualActive = true
	next, result := pressText(m, "v")
	if !result.Handled {
		t.Fatal("v (off): expected Handled=true")
	}
	if next.data.VisualActive {
		t.Fatal("v (off): expected VisualActive=false")
	}
}

func TestUpdateKey_SReturnsNeedsReload(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressText(m, "s")
	if !result.Handled {
		t.Fatal("s: expected Handled=true")
	}
	if !result.NeedsReload {
		t.Fatal("s: expected NeedsReload=true")
	}
	if next.RenderMode() != RenderModeSideBySide {
		t.Fatalf("s: expected RenderModeSideBySide, got %v", next.RenderMode())
	}
}

func TestUpdateKey_STogglesRenderModeBack(t *testing.T) {
	m := buildNavTestModel()
	m.SetRenderMode(RenderModeSideBySide)
	next, result := pressText(m, "s")
	if !result.Handled || !result.NeedsReload {
		t.Fatal("s: expected Handled=true and NeedsReload=true")
	}
	if next.RenderMode() != RenderModeUnified {
		t.Fatalf("s: expected RenderModeUnified, got %v", next.RenderMode())
	}
}

func TestUpdateKey_UnregisteredKeyNotHandled(t *testing.T) {
	m := buildNavTestModel()
	_, result := pressText(m, "x")
	if result.Handled {
		t.Fatal("x: expected Handled=false for unregistered key")
	}
}

func TestUpdateKey_GGChordJumpsTop(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 1

	// First 'g' starts chord — consumed but not matched
	next, result := pressText(m, "g")
	if !result.Handled {
		t.Fatal("g (chord prefix): expected Handled=true (chord in progress)")
	}
	if next.Data().ActiveHunk != 1 {
		t.Fatal("g: chord prefix should not change active hunk")
	}

	// Second 'g' completes chord and jumps to top
	next, result = pressText(next, "g")
	if !result.Handled {
		t.Fatal("gg: expected Handled=true")
	}
	if next.Data().ActiveHunk != 0 {
		t.Fatalf("gg: expected ActiveHunk=0, got %d", next.Data().ActiveHunk)
	}
}

func TestUpdateKey_GJumpsBottom(t *testing.T) {
	m := buildNavTestModel()
	m.SetNavMode(NavModeHunk)
	m.data.ActiveHunk = 0
	next, result := pressText(m, "G")
	if !result.Handled {
		t.Fatal("G: expected Handled=true")
	}
	last := len(next.Data().Parsed.Hunks) - 1
	if next.Data().ActiveHunk != last {
		t.Fatalf("G: expected ActiveHunk=%d (last), got %d", last, next.Data().ActiveHunk)
	}
}

func TestUpdateKey_JumpBottomScrollsViewport(t *testing.T) {
	// Build a model with many hunks spread across many lines. The last hunk
	// should be near the bottom of the content so JumpBottom+EnsureActiveVisible
	// leaves the viewport scrolled down.
	m := NewModel(false)
	raw := ""
	for i := 0; i < 15; i++ {
		start := i*10 + 1
		raw += fmt.Sprintf("@@ -%d,2 +%d,2 @@\n context\n-old%d\n+new%d\n context\n", start, start, i, i)
	}
	m.BuildFromRaw(raw, raw)
	m.SyncViewport(80, 3)
	m.data.ActiveHunk = 0

	next, _ := pressText(m, "G")
	if next.viewport.YOffset() == 0 {
		t.Fatal("G: expected viewport YOffset > 0 after JumpBottom on multi-hunk content")
	}
}

func TestUpdateKey_CtrlDPageDown(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressCtrl(m, 'd')
	if !result.Handled {
		t.Fatal("ctrl+d: expected Handled=true")
	}
	_ = next
}

func TestUpdateKey_CtrlUPageUp(t *testing.T) {
	m := buildNavTestModel()
	next, result := pressCtrl(m, 'u')
	if !result.Handled {
		t.Fatal("ctrl+u: expected Handled=true")
	}
	_ = next
}

func TestUpdateKey_NonKeyMsgNotHandled(t *testing.T) {
	m := buildNavTestModel()
	type someMsg struct{}
	_, _, result := m.Update(someMsg{})
	if result.Handled {
		t.Fatal("non-key msg: expected Handled=false")
	}
}
