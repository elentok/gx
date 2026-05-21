package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderBadgeWithColor(t *testing.T) {
	got := RenderBadgeWithColor("tag", ColorBlue, false, true)
	stripped := ansi.Strip(got)
	if !strings.Contains(stripped, "tag") {
		t.Errorf("RenderBadgeWithColor = %q, want 'tag' in output", stripped)
	}
}

func TestRenderBadgeAllVariants(t *testing.T) {
	variants := []BadgeVariant{
		BadgeVariantSurface, BadgeVariantBlue, BadgeVariantGreen,
		BadgeVariantYellow, BadgeVariantOrange, BadgeVariantMauve,
	}
	for _, v := range variants {
		got := RenderBadge("x", v, false, false)
		if ansi.Strip(got) != "x" {
			t.Errorf("RenderBadge(%q) stripped = %q, want 'x'", v, ansi.Strip(got))
		}
	}
}

func TestRenderBadgeUsesPillCaps(t *testing.T) {
	got := RenderBadge("main", BadgeVariantYellow, true, true)
	if !strings.Contains(got, capLeft) || !strings.Contains(got, capRight) {
		t.Fatalf("expected badge to render pill caps")
	}
	stripped := ansi.Strip(got)
	if !strings.Contains(stripped, " main ") {
		t.Fatalf("stripped badge = %q, want label body", stripped)
	}
}
