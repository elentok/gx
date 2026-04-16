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
