package list

// DefaultScroll is the number of lines moved by ctrl+d/ctrl+u (vim-style).
const DefaultScroll = 7

// Model holds the shared state for a list panel: selection and scroll offset.
type Model struct {
	selected     int
	scrollOffset int
}

// Selected returns the current selection index.
func (m *Model) Selected() int {
	return m.selected
}

// SetSelected sets the selection, clamped to [0, total-1]. No-op if total==0.
func (m *Model) SetSelected(i, total int) {
	if total == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i > total-1 {
		i = total - 1
	}
	m.selected = i
}

// Offset returns the current scroll offset.
func (m *Model) Offset() int {
	return m.scrollOffset
}

// Navigate moves the selection by delta (clamped to bounds), then calls
// EnsureSelectionVisible to keep the selection on screen.
func (m *Model) Navigate(delta, total, visibleH int) {
	m.SetSelected(m.selected+delta, total)
	m.EnsureSelectionVisible(total, visibleH)
}

// ScrollViewport scrolls the offset by delta (clamped to [0, max(0,total-visibleH)]),
// then snaps the selection into the visible range.
func (m *Model) ScrollViewport(delta, total, visibleH int) {
	maxOffset := total - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}

	newOffset := m.scrollOffset + delta
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOffset {
		newOffset = maxOffset
	}
	m.scrollOffset = newOffset

	// Snap selection into visible range
	if m.selected < newOffset {
		m.selected = newOffset
	}
	if visibleH > 0 && m.selected >= newOffset+visibleH {
		m.selected = newOffset + visibleH - 1
	}

	// Clamp selection to [0, total-1]
	if total > 0 {
		if m.selected < 0 {
			m.selected = 0
		}
		if m.selected > total-1 {
			m.selected = total - 1
		}
	}
}

// EnsureSelectionVisible adjusts the offset minimally to keep the selection
// on screen (no centering).
func (m *Model) EnsureSelectionVisible(total, visibleH int) {
	if m.selected < m.scrollOffset {
		m.scrollOffset = m.selected
	}
	if visibleH > 0 && m.selected >= m.scrollOffset+visibleH {
		m.scrollOffset = m.selected - visibleH + 1
	}

	// Clamp offset to [0, max(0, total-visibleH)]
	maxOffset := total - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

// VisibleRange returns the start and end indices of the visible range.
// Returns (offset, min(offset+visibleH, total)).
func (m *Model) VisibleRange(total, visibleH int) (start, end int) {
	start = m.scrollOffset
	end = m.scrollOffset + visibleH
	if end > total {
		end = total
	}
	return start, end
}

// ScrollPage moves both the selection and the viewport by delta lines (vim-style
// ctrl+d/ctrl+u: cursor and viewport move together, staying at the same screen position).
// No-op if already at the boundary in the direction of delta.
func (m *Model) ScrollPage(delta, total, visibleH int) {
	if total == 0 {
		return
	}
	if delta > 0 && m.selected >= total-1 {
		return
	}
	if delta < 0 && m.selected <= 0 {
		return
	}

	newSelected := m.selected + delta
	if newSelected < 0 {
		newSelected = 0
	}
	if newSelected > total-1 {
		newSelected = total - 1
	}

	maxOffset := total - visibleH
	if maxOffset < 0 {
		maxOffset = 0
	}
	newOffset := m.scrollOffset + delta
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOffset {
		newOffset = maxOffset
	}

	m.selected = newSelected
	m.scrollOffset = newOffset
}
