package commit

import "github.com/elentok/gx/ui"

// layout is the pane geometry for one window size. It is the single source of
// truth for the files/diff split, so the rendered rows, the scroll-clamp
// height, mouse hit-testing and the image overlay never diverge.
type layout struct {
	width int
	// bodyH is always 0: the commit header no longer reserves inline layout
	// space (it renders as a popup, see renderHeaderPopup), so diff and file
	// tree get the full height. Mouse hit-testing and the image-diff overlay
	// still use it as the row offset where the content area begins.
	bodyH    int
	contentH int
	// filesW is the files pane width in the side-by-side layout; 0 when narrow.
	filesW int
}

const narrowLayoutWidth = 90

func (m Model) layout() layout {
	l := layout{width: m.width, contentH: max(1, m.height)}
	if !l.narrow() {
		l.filesW = m.sidebar.filesPaneWidth(m.filesInput(), l.contentH)
	}
	return l
}

func (l layout) narrow() bool { return l.width < narrowLayoutWidth }

// narrowHeights splits contentH into the files and diff pane heights of the
// stacked layout, reserving one row for the seam between the panes.
func (l layout) narrowHeights() (filesH, diffH int) {
	mainH := l.contentH - 1
	filesH = max(5, mainH/3)
	diffH = max(5, mainH-filesH)
	return filesH, diffH
}

func (l layout) diffSize() (w, h int) {
	if l.narrow() {
		_, diffH := l.narrowHeights()
		return l.width, diffH
	}
	return l.width - l.filesW, l.contentH
}

func (l layout) diffRenderWidth() int {
	w, _ := l.diffSize()
	return max(1, w-4)
}

func (l layout) filesInnerHeight() int {
	if l.narrow() {
		filesH, _ := l.narrowHeights()
		return max(1, filesH-2)
	}
	return max(1, l.contentH-2)
}

func (l layout) diffRect() ui.Rect {
	if l.narrow() {
		filesH, diffH := l.narrowHeights()
		return ui.Rect{X: 0, Y: l.bodyH + filesH, W: l.width, H: diffH}
	}
	return ui.Rect{X: l.filesW, Y: l.bodyH, W: l.width - l.filesW, H: l.contentH}
}

func (l layout) filetreeRect() ui.Rect {
	if l.narrow() {
		filesH, _ := l.narrowHeights()
		return ui.Rect{X: 0, Y: l.bodyH, W: l.width, H: filesH}
	}
	return ui.Rect{X: 0, Y: l.bodyH, W: l.filesW, H: l.contentH}
}
