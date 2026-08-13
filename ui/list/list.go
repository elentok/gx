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

// uniformLineHeight treats every item as occupying exactly one screen line,
// reducing a *Lines method's line-space arithmetic to the item-space
// arithmetic its non-Lines sibling below performs directly.
func uniformLineHeight(int) int {
	return 1
}

// Navigate moves the selection by delta (clamped to bounds), then calls
// EnsureSelectionVisible to keep the selection on screen.
func (m *Model) Navigate(delta, total, visibleH int) {
	m.NavigateLines(delta, total, uniformLineHeight, visibleH)
}

// ScrollViewport scrolls the offset by delta (clamped to [0, max(0,total-visibleH)]),
// then snaps the selection into the visible range.
func (m *Model) ScrollViewport(delta, total, visibleH int) {
	m.ScrollViewportLines(delta, total, uniformLineHeight, visibleH)
}

// ScrollOffsetOnly scrolls the offset by delta (clamped to [0,
// max(0,total-visibleH)]) without snapping the selection into view, unlike
// ScrollViewport — for callers whose selection is deliberately decoupled from
// the scroll offset (e.g. a mouse wheel that pans the list without moving
// the cursor).
func (m *Model) ScrollOffsetOnly(delta, total, visibleH int) {
	m.ScrollOffsetOnlyLines(delta, total, uniformLineHeight, visibleH)
}

// EnsureSelectionVisible adjusts the offset minimally to keep the selection
// on screen (no centering).
func (m *Model) EnsureSelectionVisible(total, visibleH int) {
	m.EnsureSelectionVisibleLines(total, uniformLineHeight, visibleH)
}

// VisibleRange returns the start and end indices of the visible range.
// Returns (offset, min(offset+visibleH, total)).
func (m *Model) VisibleRange(total, visibleH int) (start, end int) {
	return m.VisibleRangeLines(total, uniformLineHeight, visibleH)
}

// ScrollPage moves both the selection and the viewport by delta lines (vim-style
// ctrl+d/ctrl+u: cursor and viewport move together, staying at the same screen position).
// No-op if already at the boundary in the direction of delta.
//
// This can't delegate to ScrollPageLines directly: that method's lineBudget
// doubles as both the paging distance and the max-offset line budget, which
// only coincide when a caller pages by exactly one viewport height. Here
// delta (paging distance) and visibleH (viewport height, bounding the max
// offset) are independent, so the item-walk uses delta as its budget while
// the offset clamp uses visibleH directly, same as the pre-Lines arithmetic.
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

	dir := 1
	if delta < 0 {
		dir = -1
	}
	itemDelta := dir * pageItemDelta(m.selected, total, uniformLineHeight, delta*dir, dir)

	newSelected := m.selected + itemDelta
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
	newOffset := m.scrollOffset + itemDelta
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOffset {
		newOffset = maxOffset
	}

	m.selected = newSelected
	m.scrollOffset = newOffset
}

// LineHeight reports how many screen lines item i occupies (minimum 1). It
// lets the *Lines methods below generalize the index-space math above to
// items of variable height, while Selected()/SetSelected() stay item-index
// space regardless. A uniform 1-line callback reduces every *Lines method to
// the identical index-space arithmetic its non-Lines sibling above already
// performs — see ui/list/list_test.go's regression suite.
type LineHeight func(i int) int

// maxLineOffset returns the largest item index the offset may sit at without
// leaving blank trailing space, given a line budget. The last item is always
// included even if it alone exceeds budget (so an over-tall final item is
// still reachable, just clipped).
func maxLineOffset(total int, lineHeight LineHeight, lineBudget int) int {
	if lineBudget <= 0 {
		return total
	}
	used, i, first := 0, total, true
	for i > 0 {
		h := lineHeight(i - 1)
		if !first && used+h > lineBudget {
			break
		}
		used += h
		i--
		first = false
	}
	return i
}

// pageItemDelta returns how many items (starting at `from`) fit within
// lineBudget lines walking in direction dir (+1 down, -1 up). Always >= 1
// when total > 0, so paging always makes progress even past an over-tall
// item.
func pageItemDelta(from, total int, lineHeight LineHeight, lineBudget, dir int) int {
	used, n, i := 0, 0, from
	for {
		h := lineHeight(i)
		if n > 0 && used+h > lineBudget {
			break
		}
		used += h
		n++
		if dir > 0 {
			i++
			if i >= total {
				break
			}
		} else {
			i--
			if i < 0 {
				break
			}
		}
	}
	return n
}

// VisibleRangeLines is the line-aware generalization of VisibleRange: it
// returns the range of items that fit within lineBudget screen lines given
// each item's lineHeight, starting at the current offset. With a uniform
// 1-line lineHeight this returns identical results to VisibleRange.
func (m *Model) VisibleRangeLines(total int, lineHeight LineHeight, lineBudget int) (start, end int) {
	start = m.scrollOffset
	used, end := 0, start
	for end < total && used < lineBudget {
		used += lineHeight(end)
		end++
	}
	return start, end
}

// EnsureSelectionVisibleLines is the line-aware generalization of
// EnsureSelectionVisible: it adjusts the offset minimally so the selected
// item's full line range is within lineBudget screen lines. With a uniform
// 1-line lineHeight this produces identical results to EnsureSelectionVisible.
func (m *Model) EnsureSelectionVisibleLines(total int, lineHeight LineHeight, lineBudget int) {
	if m.selected < m.scrollOffset {
		m.scrollOffset = m.selected
	} else if lineBudget > 0 {
		used := 0
		for i := m.scrollOffset; i <= m.selected; i++ {
			used += lineHeight(i)
		}
		if used > lineBudget {
			newOffset, span := m.selected, lineHeight(m.selected)
			for newOffset > m.scrollOffset {
				prev := newOffset - 1
				if span+lineHeight(prev) > lineBudget {
					break
				}
				span += lineHeight(prev)
				newOffset = prev
			}
			m.scrollOffset = newOffset
		}
	}

	maxOffset := maxLineOffset(total, lineHeight, lineBudget)
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

// NavigateLines is the line-aware generalization of Navigate: it moves the
// selection by delta items (clamped to bounds), then calls
// EnsureSelectionVisibleLines to keep the selection on screen.
func (m *Model) NavigateLines(delta, total int, lineHeight LineHeight, lineBudget int) {
	m.SetSelected(m.selected+delta, total)
	m.EnsureSelectionVisibleLines(total, lineHeight, lineBudget)
}

// ScrollPageLines is the line-aware generalization of ScrollPage: it pages
// both the selection and the viewport by lineBudget lines in direction dir
// (+1 down, -1 up), staying at the same screen position. A single item
// taller than lineBudget still pages by 1 item rather than stalling. With a
// uniform 1-line lineHeight and lineBudget == DefaultScroll this produces
// identical results to ScrollPage(dir*DefaultScroll, ...).
func (m *Model) ScrollPageLines(dir, total int, lineHeight LineHeight, lineBudget int) {
	if total == 0 {
		return
	}
	if dir > 0 && m.selected >= total-1 {
		return
	}
	if dir < 0 && m.selected <= 0 {
		return
	}

	delta := dir * pageItemDelta(m.selected, total, lineHeight, lineBudget, dir)

	newSelected := m.selected + delta
	if newSelected < 0 {
		newSelected = 0
	}
	if newSelected > total-1 {
		newSelected = total - 1
	}

	maxOffset := maxLineOffset(total, lineHeight, lineBudget)
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

// ScrollOffsetOnlyLines is the line-aware generalization of
// ScrollOffsetOnly: deltaLines is a line budget (not an item-space delta),
// converted to an item-count offset move via the same walk ScrollPageLines
// uses. With a uniform 1-line lineHeight this produces identical results to
// ScrollOffsetOnly.
func (m *Model) ScrollOffsetOnlyLines(deltaLines, total int, lineHeight LineHeight, lineBudget int) {
	if deltaLines == 0 {
		return
	}
	dir := 1
	if deltaLines < 0 {
		dir = -1
	}
	itemDelta := dir * pageItemDelta(m.scrollOffset, total, lineHeight, abs(deltaLines), dir)

	maxOffset := maxLineOffset(total, lineHeight, lineBudget)
	newOffset := m.scrollOffset + itemDelta
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOffset {
		newOffset = maxOffset
	}
	m.scrollOffset = newOffset
}

// ScrollViewportLines is the line-aware generalization of ScrollViewport: it
// scrolls the offset per ScrollOffsetOnlyLines, then snaps the selection
// into the resulting visible range (clip-aware, via VisibleRangeLines). With
// a uniform 1-line lineHeight this produces identical results to
// ScrollViewport.
func (m *Model) ScrollViewportLines(deltaLines, total int, lineHeight LineHeight, lineBudget int) {
	m.ScrollOffsetOnlyLines(deltaLines, total, lineHeight, lineBudget)

	if m.selected < m.scrollOffset {
		m.selected = m.scrollOffset
	}
	_, end := m.VisibleRangeLines(total, lineHeight, lineBudget)
	if end > m.scrollOffset && m.selected >= end {
		m.selected = end - 1
	}
	if total > 0 {
		if m.selected < 0 {
			m.selected = 0
		}
		if m.selected > total-1 {
			m.selected = total - 1
		}
	}
}

// ItemAtLine resolves a click's line offset (0 = first visible line, at the
// given item-index offset) to an item index, or -1 if the click landed below
// the last item (in the blank area past a short list, or on a clipped
// over-tall item's hidden tail line — those lines are never rendered, so
// bodyLine never reaches them). With a uniform 1-line lineHeight this is
// exactly offset+bodyLine, matching today's single-line click resolution.
func ItemAtLine(offset, total int, lineHeight LineHeight, bodyLine int) int {
	if bodyLine < 0 {
		return -1
	}
	used, i := 0, offset
	for i < total {
		h := lineHeight(i)
		if bodyLine < used+h {
			return i
		}
		used += h
		i++
	}
	return -1
}

func abs(n int) int {
	return max(n, -n)
}
