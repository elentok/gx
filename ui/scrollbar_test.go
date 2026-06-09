package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// scrollbarThumbRange returns the [start, end) line indices of the thumb in a
// rendered scrollbar, plus the thumb line count. start is -1 when no thumb.
func scrollbarThumbRange(t *testing.T, s string) (start, end, count int) {
	t.Helper()
	start = -1
	for i, line := range strings.Split(s, "\n") {
		if strings.Contains(ansi.Strip(line), scrollbarThumbGlyph) {
			if start == -1 {
				start = i
			}
			end = i + 1
			count++
		}
	}
	return start, end, count
}

func TestRenderScrollbar_FitsReturnsEmpty(t *testing.T) {
	if got := RenderScrollbar(10, 10, 10, 0); got != "" {
		t.Errorf("content fits exactly: got %q want empty", got)
	}
	if got := RenderScrollbar(10, 5, 10, 0); got != "" {
		t.Errorf("content smaller than viewport: got %q want empty", got)
	}
	if got := RenderScrollbar(0, 100, 10, 0); got != "" {
		t.Errorf("zero height: got %q want empty", got)
	}
}

func TestRenderScrollbar_HeightMatchesViewport(t *testing.T) {
	s := RenderScrollbar(8, 100, 8, 0)
	if lines := strings.Count(s, "\n") + 1; lines != 8 {
		t.Errorf("scrollbar has %d lines, want 8", lines)
	}
}

func TestRenderScrollbar_ThumbProportionalToVisible(t *testing.T) {
	// Half the content visible → roughly half-height thumb.
	_, _, count := scrollbarThumbRange(t, RenderScrollbar(10, 20, 10, 0))
	if count != 5 {
		t.Errorf("thumb height=%d want 5 (half of 10)", count)
	}
}

func TestRenderScrollbar_AtTop(t *testing.T) {
	start, _, count := scrollbarThumbRange(t, RenderScrollbar(10, 40, 10, 0))
	if start != 0 {
		t.Errorf("thumb at top should start at line 0, got %d", start)
	}
	if count < 1 {
		t.Fatal("expected a thumb")
	}
}

func TestRenderScrollbar_AtBottom(t *testing.T) {
	height := 10
	_, end, count := scrollbarThumbRange(t, RenderScrollbar(height, 40, 10, 30))
	if count < 1 {
		t.Fatal("expected a thumb")
	}
	if end != height {
		t.Errorf("thumb at bottom should end at line %d, got %d", height, end)
	}
}

func TestRenderScrollbar_OffsetClampedInBounds(t *testing.T) {
	height := 6
	// Offset beyond maxOffset must still land the thumb fully inside the gutter.
	start, end, count := scrollbarThumbRange(t, RenderScrollbar(height, 100, 10, 9999))
	if count < 1 {
		t.Fatal("expected a thumb")
	}
	if start < 0 || end > height {
		t.Errorf("thumb out of bounds: [%d,%d) for height %d", start, end, height)
	}
}

func TestRenderScrollbar_SmallHeightHasThumb(t *testing.T) {
	// Tiny gutter with lots of content: thumb is at least one line and in bounds.
	for _, offset := range []int{0, 50, 100} {
		start, end, count := scrollbarThumbRange(t, RenderScrollbar(2, 200, 5, offset))
		if count < 1 {
			t.Errorf("offset %d: expected at least a 1-line thumb", offset)
		}
		if start < 0 || end > 2 {
			t.Errorf("offset %d: thumb out of bounds [%d,%d) for height 2", offset, start, end)
		}
	}
}

func TestRenderScrollbar_ThumbMovesDownWithOffset(t *testing.T) {
	top, _, _ := scrollbarThumbRange(t, RenderScrollbar(10, 100, 10, 0))
	mid, _, _ := scrollbarThumbRange(t, RenderScrollbar(10, 100, 10, 45))
	bot, _, _ := scrollbarThumbRange(t, RenderScrollbar(10, 100, 10, 90))
	if !(top < mid && mid < bot) {
		t.Errorf("thumb should move down as offset grows: top=%d mid=%d bot=%d", top, mid, bot)
	}
}
