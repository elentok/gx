package list_test

import (
	"testing"

	"github.com/elentok/gx/ui/list"
)

func newModel(selected, offset int) *list.Model {
	m := &list.Model{}
	// Use Navigate to set initial state indirectly via exported methods.
	// We set via SetSelected and then force offset via ScrollViewport trick.
	// Instead, just use the zero value and navigate to position.
	_ = selected
	_ = offset
	return m
}

// helper to build a model with specific selected and offset via exported API
func buildModel(selected, offset, total, visibleH int) *list.Model {
	m := &list.Model{}
	// Scroll to the desired offset first
	if offset > 0 {
		m.ScrollViewport(offset, total, visibleH)
	}
	// Then set selection
	m.SetSelected(selected, total)
	// Ensure offset is still correct (SetSelected doesn't change offset)
	// Manually scroll if needed — we use Navigate from 0
	return m
}

// simpleModel builds a model by directly setting via Navigate from 0
func simpleModel(selected int) *list.Model {
	m := &list.Model{}
	m.SetSelected(selected, 1000)
	return m
}

func TestScrollViewport_SnapsSelectionDown(t *testing.T) {
	// Setup: 20 items, visible height 5, selection at 2, offset at 0
	// Scroll down by 5 → offset becomes 5, selection at 2 < 5, snaps to 5
	m := &list.Model{}
	m.SetSelected(2, 20)

	m.ScrollViewport(5, 20, 5)

	if m.Offset() != 5 {
		t.Errorf("expected offset=5, got %d", m.Offset())
	}
	if m.Selected() != 5 {
		t.Errorf("expected selected=5 (snapped to first visible), got %d", m.Selected())
	}
}

func TestScrollViewport_SnapsSelectionUp(t *testing.T) {
	// Setup: 20 items, visible height 5
	// Start at offset=10, selected=14 (last visible)
	// Scroll up by 5 → offset becomes 5, selected=14 >= 5+5=10, snaps to 9
	m := &list.Model{}
	m.ScrollViewport(10, 20, 5) // offset=10, selected snaps to 10
	m.SetSelected(14, 20)       // selection at bottom of viewport

	m.ScrollViewport(-5, 20, 5)

	if m.Offset() != 5 {
		t.Errorf("expected offset=5, got %d", m.Offset())
	}
	if m.Selected() != 9 {
		t.Errorf("expected selected=9 (snapped to last visible), got %d", m.Selected())
	}
}

func TestScrollViewport_LargeDeltaNoPanic(t *testing.T) {
	// Delta larger than total should not panic, just clamp
	m := &list.Model{}
	m.SetSelected(0, 10)

	// Should not panic
	m.ScrollViewport(1000, 10, 5)

	offset := m.Offset()
	selected := m.Selected()
	if offset < 0 || offset > 5 {
		t.Errorf("offset out of bounds: %d", offset)
	}
	if selected < 0 || selected > 9 {
		t.Errorf("selected out of bounds: %d", selected)
	}
}

func TestNavigate_MovesSelectionAndAdjustsOffset(t *testing.T) {
	// 20 items, visible height 5
	// Start at selection=0, offset=0
	// Navigate by 7 → selection=7, offset adjusts to show it (7 >= 0+5, so offset=7-5+1=3)
	m := &list.Model{}

	m.Navigate(7, 20, 5)

	if m.Selected() != 7 {
		t.Errorf("expected selected=7, got %d", m.Selected())
	}
	if m.Offset() != 3 {
		t.Errorf("expected offset=3, got %d", m.Offset())
	}
}

func TestNavigate_ClampsToBounds(t *testing.T) {
	m := &list.Model{}
	m.SetSelected(18, 20)

	// Navigate past end
	m.Navigate(5, 20, 5)

	if m.Selected() != 19 {
		t.Errorf("expected selected=19 (clamped), got %d", m.Selected())
	}
}

func TestEnsureSelectionVisible_AdjustsOffsetMinimally(t *testing.T) {
	// 20 items, visible height 5
	// offset=5, selection=3 (above viewport) → offset should move to 3
	m := &list.Model{}
	m.ScrollViewport(5, 20, 5) // offset=5
	m.SetSelected(3, 20)       // selection above viewport

	m.EnsureSelectionVisible(20, 5)

	if m.Offset() != 3 {
		t.Errorf("expected offset=3 (minimal adjustment), got %d", m.Offset())
	}
}

func TestEnsureSelectionVisible_NoCentering(t *testing.T) {
	// 20 items, visible height 5
	// offset=0, selection=6 (below viewport: 0+5=5) → offset = 6-5+1 = 2
	m := &list.Model{}
	m.SetSelected(6, 20)

	m.EnsureSelectionVisible(20, 5)

	if m.Offset() != 2 {
		t.Errorf("expected offset=2 (not centered), got %d", m.Offset())
	}
}

func TestEdgeCase_TotalZero(t *testing.T) {
	m := &list.Model{}

	// SetSelected with total=0 is a no-op
	m.SetSelected(5, 0)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0 for total=0, got %d", m.Selected())
	}

	// Navigate with total=0 should not panic
	m.Navigate(1, 0, 5)

	// ScrollViewport with total=0 should not panic
	m.ScrollViewport(1, 0, 5)

	// EnsureSelectionVisible with total=0 should not panic
	m.EnsureSelectionVisible(0, 5)

	// VisibleRange with total=0
	start, end := m.VisibleRange(0, 5)
	if start != 0 || end != 0 {
		t.Errorf("expected (0,0) for total=0, got (%d,%d)", start, end)
	}
}

func TestEdgeCase_VisibleHZero(t *testing.T) {
	m := &list.Model{}
	m.SetSelected(5, 20)

	// EnsureSelectionVisible with visibleH=0 should not adjust offset into negative
	m.EnsureSelectionVisible(20, 0)

	// ScrollViewport with visibleH=0 should not panic
	m.ScrollViewport(1, 20, 0)

	// Navigate with visibleH=0 should not panic
	m.Navigate(1, 20, 0)
}

func TestEdgeCase_SingleItem(t *testing.T) {
	m := &list.Model{}

	m.SetSelected(0, 1)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0, got %d", m.Selected())
	}

	m.Navigate(1, 1, 5)
	if m.Selected() != 0 {
		t.Errorf("expected selected=0 (clamped), got %d", m.Selected())
	}

	m.ScrollViewport(5, 1, 5)
	if m.Offset() != 0 {
		t.Errorf("expected offset=0 for single item, got %d", m.Offset())
	}

	start, end := m.VisibleRange(1, 5)
	if start != 0 || end != 1 {
		t.Errorf("expected (0,1), got (%d,%d)", start, end)
	}
}

func TestVisibleRange(t *testing.T) {
	m := &list.Model{}
	m.ScrollViewport(3, 20, 5) // offset=3

	start, end := m.VisibleRange(20, 5)
	if start != 3 {
		t.Errorf("expected start=3, got %d", start)
	}
	if end != 8 {
		t.Errorf("expected end=8, got %d", end)
	}
}

func TestVisibleRange_ClipsAtTotal(t *testing.T) {
	m := &list.Model{}
	m.ScrollViewport(18, 20, 5) // offset clamped to 15

	start, end := m.VisibleRange(20, 5)
	if start != 15 {
		t.Errorf("expected start=15, got %d", start)
	}
	if end != 20 {
		t.Errorf("expected end=20, got %d", end)
	}
}
