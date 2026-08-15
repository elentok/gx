package ui

// Rect is an axis-aligned rectangle. Bounds are half-open on both axes: a
// point matches when it falls in [X, X+W) horizontally and [Y, Y+H)
// vertically, matching every hand-rolled hit-test in the codebase.
type Rect struct {
	X, Y, W, H int
}

// Contains reports whether (x, y) falls within r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// HoverHitTest returns the index of the first rect in rects containing
// (x, y). ok is false when (x, y) falls in a gap covered by none of the
// candidates — a seam, a border, any point covered by no rect — in which
// case the caller's wheel event should be a no-op, not a fallback to any
// other rect.
func HoverHitTest(x, y int, rects ...Rect) (index int, ok bool) {
	for i, r := range rects {
		if r.Contains(x, y) {
			return i, true
		}
	}
	return -1, false
}
