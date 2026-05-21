package ui

import (
	"image/color"
	"strings"
	"testing"
)

func TestRenderRowHighlight_AddsBackground(t *testing.T) {
	out := RenderRowHighlight("hello")
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI escape in RenderRowHighlight output")
	}
}

func TestRenderRowWithBackground_AppliesAfterReset(t *testing.T) {
	input := "before\x1b[0mafter"
	out := RenderRowWithBackground(input, color.RGBA{R: 30, G: 30, B: 30, A: 255})
	// The reset should be followed by a background sequence
	if !strings.Contains(out, "\x1b[0m\x1b[48;2;") {
		t.Errorf("expected background re-applied after reset, got %q", out)
	}
}

func TestBackgroundANSI_Format(t *testing.T) {
	out := backgroundANSI(color.RGBA{R: 10, G: 20, B: 30, A: 255})
	expected := "\x1b[48;2;10;20;30m"
	if out != expected {
		t.Errorf("backgroundANSI() = %q, want %q", out, expected)
	}
}
