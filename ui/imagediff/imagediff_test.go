package imagediff_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/elentok/gx/ui/imagediff"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func encodeGIF(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.White, color.Black})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("gif.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestPlan_BothSidesPresent(t *testing.T) {
	t.Parallel()
	old := encodePNG(t, 120, 80)
	new_ := encodePNG(t, 200, 80) // resized: different aspect ratio than old

	plan := imagediff.Plan(old, new_, 80, 24, 16, 30)

	if plan.Fallback {
		t.Fatalf("expected no fallback, got plan: %+v", plan)
	}
	if plan.Layout != imagediff.LayoutSideBySide {
		t.Fatalf("Layout = %v, want LayoutSideBySide", plan.Layout)
	}
	if plan.Old == nil || plan.New == nil {
		t.Fatalf("expected both placements, got Old=%v New=%v", plan.Old, plan.New)
	}
	if plan.Old.SpanCols <= 0 || plan.Old.SpanRows <= 0 {
		t.Errorf("Old span must be positive, got %+v", plan.Old)
	}
	if plan.New.SpanCols <= 0 || plan.New.SpanRows <= 0 {
		t.Errorf("New span must be positive, got %+v", plan.New)
	}
	// New is wider (200x80 vs 120x80) so, scaled to fit the same half-width box
	// at the same pixel-per-cell ratio, it should occupy fewer rows than Old.
	if plan.New.SpanRows >= plan.Old.SpanRows {
		t.Errorf("expected New (wider aspect) to span fewer rows than Old: New=%+v Old=%+v", plan.New, plan.Old)
	}
	// Each pane must fit within its half of the available width.
	halfCols := (80 - 2) / 2
	if plan.Old.SpanCols > halfCols || plan.New.SpanCols > halfCols {
		t.Errorf("spans exceed half-width %d: Old=%d New=%d", halfCols, plan.Old.SpanCols, plan.New.SpanCols)
	}
	// New's pane must start to the right of Old's pane (no overlap).
	if plan.New.Col < plan.Old.Col+plan.Old.SpanCols {
		t.Errorf("expected New pane to start after Old pane: Old.Col=%d+%d, New.Col=%d", plan.Old.Col, plan.Old.SpanCols, plan.New.Col)
	}
}

func TestPlan_AddedFile(t *testing.T) {
	t.Parallel()
	new_ := encodePNG(t, 100, 50)

	plan := imagediff.Plan(nil, new_, 80, 24, 16, 30)

	if plan.Fallback {
		t.Fatalf("expected no fallback, got plan: %+v", plan)
	}
	if plan.Layout != imagediff.LayoutCentered {
		t.Fatalf("Layout = %v, want LayoutCentered", plan.Layout)
	}
	if plan.Old != nil {
		t.Errorf("expected no Old placement for added file, got %+v", plan.Old)
	}
	if plan.New == nil || plan.New.SpanCols <= 0 || plan.New.SpanRows <= 0 {
		t.Fatalf("expected positive New placement, got %+v", plan.New)
	}
	if plan.New.SpanCols > 80 || plan.New.SpanRows > 24 {
		t.Errorf("New placement exceeds available space: %+v", plan.New)
	}
}

func TestPlan_DeletedFile(t *testing.T) {
	t.Parallel()
	old := encodePNG(t, 100, 50)

	plan := imagediff.Plan(old, nil, 80, 24, 16, 30)

	if plan.Fallback {
		t.Fatalf("expected no fallback, got plan: %+v", plan)
	}
	if plan.Layout != imagediff.LayoutCentered {
		t.Fatalf("Layout = %v, want LayoutCentered", plan.Layout)
	}
	if plan.New != nil {
		t.Errorf("expected no New placement for deleted file, got %+v", plan.New)
	}
	if plan.Old == nil || plan.Old.SpanCols <= 0 || plan.Old.SpanRows <= 0 {
		t.Fatalf("expected positive Old placement, got %+v", plan.Old)
	}
}

func TestPlan_DecodeFailureFallsBack(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		old, new_ []byte
	}{
		"both corrupt": {
			old:  []byte("not an image"),
			new_: []byte("also not an image"),
		},
		"old corrupt, new valid": {
			old:  []byte("garbage"),
			new_: encodePNG(t, 50, 50),
		},
		"old valid, new corrupt": {
			old:  encodePNG(t, 50, 50),
			new_: []byte("garbage"),
		},
		"both empty": {
			old:  nil,
			new_: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			plan := imagediff.Plan(tc.old, tc.new_, 80, 24, 16, 30)
			if !plan.Fallback {
				t.Fatalf("expected fallback, got plan: %+v", plan)
			}
		})
	}
}

func TestPlan_OversizedInputFallsBack(t *testing.T) {
	t.Parallel()
	huge := make([]byte, 9*1024*1024)

	plan := imagediff.Plan(huge, nil, 80, 24, 16, 30)

	if !plan.Fallback {
		t.Fatalf("expected fallback for oversized input, got plan: %+v", plan)
	}
}

func TestPlan_AspectRatioEdgeCases(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		w, h int
	}{
		"very wide": {w: 1000, h: 50},
		"very tall": {w: 50, h: 1000},
		"square":    {w: 100, h: 100},
		"tiny":      {w: 1, h: 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			img := encodePNG(t, tc.w, tc.h)

			plan := imagediff.Plan(img, nil, 80, 24, 16, 30)

			if plan.Fallback {
				t.Fatalf("expected no fallback, got plan: %+v", plan)
			}
			if plan.Old == nil {
				t.Fatalf("expected Old placement")
			}
			if plan.Old.SpanCols < 1 || plan.Old.SpanRows < 1 {
				t.Errorf("span must be at least 1x1, got %+v", plan.Old)
			}
			if plan.Old.SpanCols > 80 || plan.Old.SpanRows > 24 {
				t.Errorf("span exceeds available space: %+v", plan.Old)
			}
		})
	}
}

func TestPlan_FormatsDecode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		encode func(t *testing.T) []byte
	}{
		"png":  {encode: func(t *testing.T) []byte { return encodePNG(t, 40, 30) }},
		"jpeg": {encode: func(t *testing.T) []byte { return encodeJPEG(t, 40, 30) }},
		"gif":  {encode: func(t *testing.T) []byte { return encodeGIF(t, 40, 30) }},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := tc.encode(t)

			plan := imagediff.Plan(data, nil, 80, 24, 16, 30)

			if plan.Fallback {
				t.Fatalf("expected %s to decode without fallback, got plan: %+v", name, plan)
			}
			if plan.Old == nil || plan.Old.SpanCols < 1 || plan.Old.SpanRows < 1 {
				t.Fatalf("expected positive Old placement for %s, got %+v", name, plan.Old)
			}
		})
	}
}
