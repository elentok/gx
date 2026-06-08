package splitview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// --- Mock panels ---

type mockList struct {
	ref string
}

func (m mockList) Init() tea.Cmd                           { return nil }
func (m mockList) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m mockList) View() tea.View                          { return tea.NewView("list") }
func (m mockList) SelectedRef() string                     { return m.ref }

type mockDetail struct{}

func (m mockDetail) Init() tea.Cmd                           { return nil }
func (m mockDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m mockDetail) View() tea.View                          { return tea.NewView("detail") }

func newTestModel(w, h int) Model {
	return Model{
		list:       mockList{ref: "abc"},
		detail:     mockDetail{},
		vis:        visModeCollapsed,
		focus:      focusList,
		width:      w,
		height:     h,
		autoOrient: true,
	}
}

func pressKey(text string) tea.KeyPressMsg {
	if len(text) == 1 {
		return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
	}
	switch text {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	}
	return tea.KeyPressMsg{Text: text}
}

// --- State machine transitions ---

func TestCollapsedEnterExpandsToSplitDetailFocused(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m, _ = m.Update(pressKey("enter"))
	if !m.IsSplit() {
		t.Fatal("expected Split after enter in Collapsed")
	}
	if !m.IsDetailFocused() {
		t.Fatal("expected detail focused after expanding")
	}
}

func TestSplitDetailFocusedEscFocusesList(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusDetail
	m, _ = m.Update(pressKey("esc"))
	if !m.IsSplit() {
		t.Fatal("expected still in Split state")
	}
	if !m.IsListFocused() {
		t.Fatal("expected list focused after esc from detail")
	}
}

func TestSplitDetailFocusedQFocusesList(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusDetail
	m, _ = m.Update(pressKey("q"))
	if !m.IsSplit() {
		t.Fatal("expected still in Split state")
	}
	if !m.IsListFocused() {
		t.Fatal("expected list focused after q from detail")
	}
}

func TestSplitListFocusedEscCollapse(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusList
	m, _ = m.Update(pressKey("esc"))
	if !m.IsCollapsed() {
		t.Fatal("expected Collapsed after esc from list in Split")
	}
}

func TestSplitListFocusedQCollapse(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusList
	m, _ = m.Update(pressKey("q"))
	if !m.IsCollapsed() {
		t.Fatal("expected Collapsed after q from list in Split")
	}
}

func TestEnterOnSplitListFocusedFocusesDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusList
	m, _ = m.Update(pressKey("enter"))
	if !m.IsSplit() {
		t.Fatal("expected still in Split")
	}
	if !m.IsDetailFocused() {
		t.Fatal("expected detail focused after enter in Split list focused")
	}
}

func TestFullscreenFromCollapsed(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	// Collapsed, list focused → f → fullscreen list
	m, _ = m.Update(pressKey("f"))
	if !m.IsFullscreen() {
		t.Fatal("expected Fullscreen after f in Collapsed")
	}
	if !m.IsListFocused() {
		t.Fatal("expected list focused in fullscreen (was list focused before)")
	}
}

func TestFullscreenFromSplitDetailFocused(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusDetail
	m, _ = m.Update(pressKey("f"))
	if !m.IsFullscreen() {
		t.Fatal("expected Fullscreen after f")
	}
	if !m.IsDetailFocused() {
		t.Fatal("expected detail focused in fullscreen")
	}
}

func TestFullscreenFRestoresPreviousState(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	m.focus = focusDetail
	// Enter fullscreen
	m, _ = m.Update(pressKey("f"))
	if !m.IsFullscreen() {
		t.Fatal("expected Fullscreen")
	}
	// Exit fullscreen
	m, _ = m.Update(pressKey("f"))
	if m.IsFullscreen() {
		t.Fatal("expected to exit fullscreen after second f")
	}
	if !m.IsSplit() {
		t.Fatal("expected Split restored")
	}
	if !m.IsDetailFocused() {
		t.Fatal("expected detail focused restored")
	}
}

func TestFullscreenFromCollapsedRestoresCollapsed(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	// Collapsed → fullscreen → back
	m, _ = m.Update(pressKey("f"))
	m, _ = m.Update(pressKey("f"))
	if !m.IsCollapsed() {
		t.Fatal("expected Collapsed restored after f+f from Collapsed")
	}
}

// --- Layout sizes ---

func TestCollapsedSizes(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	if lw != 200 || lh != 50 {
		t.Fatalf("list size: got (%d,%d), want (200,50)", lw, lh)
	}
	if dw != 0 || dh != 0 {
		t.Fatalf("detail size: got (%d,%d), want (0,0)", dw, dh)
	}
}

func TestSplitVerticalSizes(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	// width=200 > 100 → Vertical auto-orientation
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	expectedListW := int(float64(200) * 0.40) // 80
	if lw != expectedListW {
		t.Fatalf("list width: got %d, want %d", lw, expectedListW)
	}
	if lh != 50 {
		t.Fatalf("list height: got %d, want 50", lh)
	}
	if dw != 200-expectedListW {
		t.Fatalf("detail width: got %d, want %d", dw, 200-expectedListW)
	}
	if dh != 50 {
		t.Fatalf("detail height: got %d, want 50", dh)
	}
}

func TestSplitHorizontalSizes(t *testing.T) {
	t.Parallel()
	m := newTestModel(80, 40)
	m.vis = visModeSplit
	// width=80 ≤ 100 → Horizontal auto-orientation
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	expectedListH := int(float64(40) * 0.30) // 12
	if lw != 80 {
		t.Fatalf("list width: got %d, want 80", lw)
	}
	if lh != expectedListH {
		t.Fatalf("list height: got %d, want %d", lh, expectedListH)
	}
	if dw != 80 {
		t.Fatalf("detail width: got %d, want 80", dw)
	}
	if dh != 40-expectedListH {
		t.Fatalf("detail height: got %d, want %d", dh, 40-expectedListH)
	}
}

func TestFullscreenListSizes(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeFullscreen
	m.focus = focusList
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	if lw != 200 || lh != 50 {
		t.Fatalf("list size: got (%d,%d), want (200,50)", lw, lh)
	}
	if dw != 0 || dh != 0 {
		t.Fatalf("detail size: got (%d,%d), want (0,0)", dw, dh)
	}
}

func TestFullscreenDetailSizes(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeFullscreen
	m.focus = focusDetail
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	if lw != 0 || lh != 0 {
		t.Fatalf("list size: got (%d,%d), want (0,0)", lw, lh)
	}
	if dw != 200 || dh != 50 {
		t.Fatalf("detail size: got (%d,%d), want (200,50)", dw, dh)
	}
}

// --- Detail origin ---

func TestDetailOriginCollapsedNotVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeCollapsed
	if _, _, visible := m.DetailOrigin(); visible {
		t.Fatalf("expected detail not visible when collapsed")
	}
}

func TestDetailOriginVerticalSplit(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeSplit
	col, row, visible := m.DetailOrigin()
	if !visible {
		t.Fatalf("expected detail visible in split")
	}
	expectedCol := int(float64(200) * 0.40) // list width = 80
	if col != expectedCol || row != 0 {
		t.Fatalf("origin: got (%d,%d), want (%d,0)", col, row, expectedCol)
	}
}

func TestDetailOriginHorizontalSplit(t *testing.T) {
	t.Parallel()
	m := newTestModel(80, 40)
	m.vis = visModeSplit
	col, row, visible := m.DetailOrigin()
	if !visible {
		t.Fatalf("expected detail visible in split")
	}
	expectedRow := int(float64(40) * 0.30) // list height = 12
	if col != 0 || row != expectedRow {
		t.Fatalf("origin: got (%d,%d), want (0,%d)", col, row, expectedRow)
	}
}

func TestDetailOriginFullscreenList(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeFullscreen
	m.focus = focusList
	if _, _, visible := m.DetailOrigin(); visible {
		t.Fatalf("expected detail not visible when list is fullscreen")
	}
}

func TestDetailOriginFullscreenDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	m.vis = visModeFullscreen
	m.focus = focusDetail
	col, row, visible := m.DetailOrigin()
	if !visible || col != 0 || row != 0 {
		t.Fatalf("origin: got (%d,%d,%v), want (0,0,true)", col, row, visible)
	}
}

// --- Auto-orientation threshold ---

func TestAutoOrientationHorizontalAtWidth99(t *testing.T) {
	t.Parallel()
	m := newTestModel(99, 40)
	if m.effectiveOrientation() != Horizontal {
		t.Fatal("expected Horizontal at width 99")
	}
}

func TestAutoOrientationHorizontalAtWidth100(t *testing.T) {
	t.Parallel()
	m := newTestModel(100, 40)
	if m.effectiveOrientation() != Horizontal {
		t.Fatal("expected Horizontal at width 100")
	}
}

func TestAutoOrientationVerticalAtWidth101(t *testing.T) {
	t.Parallel()
	m := newTestModel(101, 40)
	if m.effectiveOrientation() != Vertical {
		t.Fatal("expected Vertical at width 101")
	}
}

// --- Orientation toggle ---

func TestOrientationToggleDisablesAutoAndSwitches(t *testing.T) {
	t.Parallel()
	// Start with auto (vertical at width 200).
	m := newTestModel(200, 50)
	if m.effectiveOrientation() != Vertical {
		t.Fatal("expected Vertical auto-orientation at width 200")
	}
	// Send "to" chord to toggle.
	m, _ = m.Update(pressKey("t"))
	m, _ = m.Update(pressKey("o"))
	if m.autoOrient {
		t.Fatal("expected autoOrient to be disabled after toggle")
	}
	// Starting orientation field was Vertical (zero value), so after toggle it should be Horizontal.
	if m.orientation != Horizontal {
		t.Fatal("expected orientation toggled to Horizontal")
	}
	if m.effectiveOrientation() != Horizontal {
		t.Fatal("expected effectiveOrientation to be Horizontal after toggle")
	}
}

func TestOrientationToggleSecondTimeSwapsBack(t *testing.T) {
	t.Parallel()
	m := newTestModel(200, 50)
	// First toggle: Vertical → Horizontal.
	m, _ = m.Update(pressKey("t"))
	m, _ = m.Update(pressKey("o"))
	// Second toggle: Horizontal → Vertical.
	m, _ = m.Update(pressKey("t"))
	m, _ = m.Update(pressKey("o"))
	if m.orientation != Vertical {
		t.Fatal("expected orientation toggled back to Vertical")
	}
}
