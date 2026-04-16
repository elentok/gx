package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderModalFrameIncludesTitleBodyAndHint(t *testing.T) {
	out := RenderModalFrame(ModalFrameOptions{
		Title: "Title",
		Body:  "Body line",
		Hint:  "Hint text",
		Width: 24,
	})

	if !strings.Contains(out, "Title") {
		t.Fatalf("expected title in modal frame: %q", out)
	}
	if !strings.Contains(out, "Body line") {
		t.Fatalf("expected body in modal frame: %q", out)
	}
	if !strings.Contains(out, "Hint text") {
		t.Fatalf("expected hint in modal frame: %q", out)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected bordered multi-line modal, got %d lines: %q", len(lines), out)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 24 {
			t.Fatalf("line %d width = %d, want 24: %q", i, got, line)
		}
	}
}

func TestRenderPanelFrameReturnsEmptyForTooSmallDimensions(t *testing.T) {
	if got := RenderPanelFrame(PanelFrameOptions{Width: 1, Height: 3}); got != "" {
		t.Fatalf("expected empty frame for narrow dimensions, got %q", got)
	}
	if got := RenderPanelFrame(PanelFrameOptions{Width: 3, Height: 1}); got != "" {
		t.Fatalf("expected empty frame for short dimensions, got %q", got)
	}
}

func TestRenderPanelFrameUsesFixedDimensions(t *testing.T) {
	out := RenderPanelFrame(PanelFrameOptions{
		Width:      14,
		Height:     5,
		Title:      "Left",
		RightTitle: "Right",
		Lines:      []string{"alpha", "beta"},
	})

	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5: %q", len(lines), out)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 14 {
			t.Fatalf("line %d width = %d, want 14: %q", i, got, line)
		}
	}
	if !strings.Contains(out, "Left") {
		t.Fatalf("expected left title in panel frame: %q", out)
	}
	if !strings.Contains(out, "Righ") {
		t.Fatalf("expected right title content in panel frame: %q", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("expected body lines in panel frame: %q", out)
	}
}

func TestRenderPanelFrameTruncatesLongBodyLines(t *testing.T) {
	out := RenderPanelFrame(PanelFrameOptions{
		Width:  8,
		Height: 3,
		Lines:  []string{"abcdefghijk"},
	})

	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3: %q", len(lines), out)
	}
	if strings.Contains(lines[1], "g") {
		t.Fatalf("expected body line truncation within inner width, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "abcdef") {
		t.Fatalf("expected truncated body content to keep visible prefix, got %q", lines[1])
	}
}
