package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/keys"
)

func TestChordBindingsFromHints(t *testing.T) {
	hints := []keys.ChordHint{
		{Key: "t", Desc: "jump to tag"},
		{Key: "h", Desc: "jump to HEAD"},
	}
	bindings := ChordBindingsFromHints(hints)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	if bindings[0].Title != "jump to tag" {
		t.Errorf("binding[0].Title = %q, want 'jump to tag'", bindings[0].Title)
	}
}

func TestRenderChordOverlay_NonEmpty(t *testing.T) {
	bindings := []keys.Binding{
		{Seq: []string{"t"}, Title: "jump to tag"},
		{Seq: []string{"h"}, Title: "jump to HEAD"},
	}
	got := RenderChordOverlay("]", bindings)
	if got == "" {
		t.Error("expected non-empty chord overlay")
	}
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "jump to tag") {
		t.Errorf("expected 'jump to tag' in overlay: %q", plain)
	}
}

func TestRenderChordOverlay_Empty(t *testing.T) {
	got := RenderChordOverlay("]", nil)
	if got != "" {
		t.Errorf("expected empty overlay for nil bindings, got %q", got)
	}
}
