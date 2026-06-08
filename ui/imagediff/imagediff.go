// Package imagediff is a pure layout module for the inline image-diff feature.
// It takes raw old/new image bytes plus the available terminal cell space and
// produces a RenderPlan describing how (or whether) to lay the comparison out —
// no terminal I/O, no git knowledge, no bubbletea knowledge.
package imagediff

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// maxCombinedBytes is the size cap above which we fall back rather than decode —
// large images are slow to decode and unlikely to render usefully in a terminal cell grid.
const maxCombinedBytes = 8 * 1024 * 1024

// gutterCols is the blank column gap left between the old/new panes in a side-by-side layout.
const gutterCols = 2

// Layout describes how the old/new images are arranged within the available space.
type Layout int

const (
	// LayoutFallback means the caller should render binarySummaryLine() instead.
	LayoutFallback Layout = iota
	// LayoutSideBySide places both images side by side, each scaled to its own aspect ratio.
	LayoutSideBySide
	// LayoutCentered places a single image centered in the available space (added/deleted file).
	LayoutCentered
)

// Placement describes where and at what cell-span an image should be rendered,
// relative to the top-left corner of the available area.
type Placement struct {
	Col, Row int
	SpanCols int
	SpanRows int
}

// RenderPlan is the layout decision for a single image-diff comparison.
type RenderPlan struct {
	Fallback bool
	Layout   Layout

	// Old/New are nil when that side isn't present (added/deleted file) or the
	// overall plan is a fallback.
	Old *Placement
	New *Placement
}

// Plan decodes old and new image bytes and decides how to lay out a side-by-side
// (or single centered) comparison within availCols x availRows terminal cells,
// given the host's pixel-per-cell ratio (pxPerCol x pxPerRow).
//
// Whenever either side fails to decode, both sides are absent, or the combined
// input exceeds the size cap, Plan returns a fallback plan — the caller's job
// reduces to a single `if plan.Fallback { show binarySummaryLine() }` branch.
func Plan(old, new []byte, availCols, availRows int, pxPerCol, pxPerRow float64) RenderPlan {
	if len(old)+len(new) > maxCombinedBytes {
		return RenderPlan{Fallback: true}
	}
	if availCols <= 0 || availRows <= 0 || pxPerCol <= 0 || pxPerRow <= 0 {
		return RenderPlan{Fallback: true}
	}

	// An empty side is the expected, common "not present" state (added/deleted
	// file); a non-empty side that fails to decode means corrupt/unsupported
	// data, which is a fallback trigger per the spec.
	oldImg, oldOK, oldCorrupt := decode(old)
	newImg, newOK, newCorrupt := decode(new)

	if oldCorrupt || newCorrupt {
		return RenderPlan{Fallback: true}
	}
	if !oldOK && !newOK {
		return RenderPlan{Fallback: true}
	}

	if oldOK && newOK {
		return planSideBySide(oldImg, newImg, availCols, availRows, pxPerCol, pxPerRow)
	}

	if oldOK {
		return planCentered(oldImg, availCols, availRows, pxPerCol, pxPerRow, true)
	}
	return planCentered(newImg, availCols, availRows, pxPerCol, pxPerRow, false)
}

// decode reports, in order: the decoded image (if any), whether this side is
// present and usable, and whether this side is present but corrupt/undecodable
// (a fallback trigger, distinct from simply being absent).
func decode(data []byte) (img image.Image, ok bool, corrupt bool) {
	if len(data) == 0 {
		return nil, false, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, true
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, false, true
	}
	return img, true, false
}

func planSideBySide(oldImg, newImg image.Image, availCols, availRows int, pxPerCol, pxPerRow float64) RenderPlan {
	halfCols := (availCols - gutterCols) / 2
	if halfCols < 1 {
		halfCols = availCols / 2
	}
	if halfCols < 1 {
		return RenderPlan{Fallback: true}
	}

	oldBounds := oldImg.Bounds()
	newBounds := newImg.Bounds()

	oldSpanCols, oldSpanRows := fitSpan(oldBounds.Dx(), oldBounds.Dy(), halfCols, availRows, pxPerCol, pxPerRow)
	newSpanCols, newSpanRows := fitSpan(newBounds.Dx(), newBounds.Dy(), halfCols, availRows, pxPerCol, pxPerRow)

	rightStart := availCols - halfCols

	return RenderPlan{
		Layout: LayoutSideBySide,
		Old: &Placement{
			Col:      centerOffset(halfCols, oldSpanCols),
			Row:      centerOffset(availRows, oldSpanRows),
			SpanCols: oldSpanCols,
			SpanRows: oldSpanRows,
		},
		New: &Placement{
			Col:      rightStart + centerOffset(halfCols, newSpanCols),
			Row:      centerOffset(availRows, newSpanRows),
			SpanCols: newSpanCols,
			SpanRows: newSpanRows,
		},
	}
}

func planCentered(img image.Image, availCols, availRows int, pxPerCol, pxPerRow float64, isOld bool) RenderPlan {
	bounds := img.Bounds()
	spanCols, spanRows := fitSpan(bounds.Dx(), bounds.Dy(), availCols, availRows, pxPerCol, pxPerRow)

	placement := &Placement{
		Col:      centerOffset(availCols, spanCols),
		Row:      centerOffset(availRows, spanRows),
		SpanCols: spanCols,
		SpanRows: spanRows,
	}

	plan := RenderPlan{Layout: LayoutCentered}
	if isOld {
		plan.Old = placement
	} else {
		plan.New = placement
	}
	return plan
}

// fitSpan chooses the integer column/row span that best preserves the image's
// true pixel aspect ratio while fitting within maxCols x maxRows, given the
// host's pixel-per-cell ratio (cells are typically ~2x taller than wide).
func fitSpan(pxW, pxH, maxCols, maxRows int, pxPerCol, pxPerRow float64) (cols, rows int) {
	if maxCols < 1 {
		maxCols = 1
	}
	if maxRows < 1 {
		maxRows = 1
	}

	// Largest uniform pixel scale that fits the image within the available
	// terminal-cell pixel area, preserving its true aspect ratio.
	scale := minFloat(
		float64(maxCols)*pxPerCol/float64(pxW),
		float64(maxRows)*pxPerRow/float64(pxH),
	)

	cols = roundToAtLeastOne(float64(pxW) * scale / pxPerCol)
	rows = roundToAtLeastOne(float64(pxH) * scale / pxPerRow)

	if cols > maxCols {
		cols = maxCols
	}
	if rows > maxRows {
		rows = maxRows
	}
	return cols, rows
}

func centerOffset(available, span int) int {
	offset := (available - span) / 2
	if offset < 0 {
		return 0
	}
	return offset
}

func roundToAtLeastOne(v float64) int {
	rounded := int(v + 0.5)
	if rounded < 1 {
		return 1
	}
	return rounded
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
