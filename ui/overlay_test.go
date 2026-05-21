package ui

import (
	"strings"
	"testing"
)

func TestPlaceOverlayReplacesBackgroundAtPosition(t *testing.T) {
	bg := strings.Join([]string{
		"abcdef",
		"ghijkl",
		"mnopqr",
	}, "\n")

	got := PlaceOverlay(bg, "XY", 2, 1)
	want := strings.Join([]string{
		"abcdef",
		"ghXYkl",
		"mnopqr",
	}, "\n")

	if got != want {
		t.Fatalf("PlaceOverlay() = %q, want %q", got, want)
	}
}

func TestPlaceOverlayPadsShortBackgroundLines(t *testing.T) {
	bg := strings.Join([]string{
		"ab",
		"cd",
	}, "\n")

	got := PlaceOverlay(bg, "XYZ", 4, 0)
	want := strings.Join([]string{
		"ab  XYZ",
		"cd",
	}, "\n")

	if got != want {
		t.Fatalf("PlaceOverlay() = %q, want %q", got, want)
	}
}

func TestPlaceOverlayIgnoresRowsOutsideBackground(t *testing.T) {
	bg := strings.Join([]string{
		"one",
		"two",
	}, "\n")

	got := PlaceOverlay(bg, "A\nB", 1, -1)
	want := strings.Join([]string{
		"oBe",
		"two",
	}, "\n")

	if got != want {
		t.Fatalf("PlaceOverlay() = %q, want %q", got, want)
	}
}

func TestOverlayCenterCentersForeground(t *testing.T) {
	bg := strings.Join([]string{
		"......",
		"......",
		"......",
		"......",
	}, "\n")

	got := OverlayCenter(bg, "XX\nYY", 6, 4)
	want := strings.Join([]string{
		"......",
		"..XX..",
		"..YY..",
		"......",
	}, "\n")

	if got != want {
		t.Fatalf("OverlayCenter() = %q, want %q", got, want)
	}
}

func TestOverlayBottomCenter(t *testing.T) {
	bg := strings.Join([]string{"......", "......", "......", "......", "......"}, "\n")
	got := OverlayBottomCenter(bg, "XX", 6, 3)
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[3], "XX") {
		t.Errorf("expected 'XX' on line 3, got %q", lines[3])
	}
}

func TestOverlayTopRight(t *testing.T) {
	bg := strings.Join([]string{"......", "......", "......"}, "\n")
	got := OverlayTopRight(bg, "XX", 6)
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[1], "XX") {
		t.Errorf("expected 'XX' on line 1, got %q", lines[1])
	}
}

func TestOverlayBottomRight(t *testing.T) {
	bg := strings.Join([]string{"......", "......", "......", "......"}, "\n")
	got := OverlayBottomRight(bg, "XX", 6, 4)
	_ = got // just verify no panic
}

func TestOverlayTopRightMargin(t *testing.T) {
	bg := strings.Join([]string{"......", "......", "......"}, "\n")
	got := OverlayTopRightMargin(bg, "XX", 6, 1, 0)
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "XX") {
		t.Errorf("expected 'XX' on line 0, got %q", lines[0])
	}
}
