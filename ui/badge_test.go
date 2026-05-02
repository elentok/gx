package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderBadgeUsesPillCaps(t *testing.T) {
	got := RenderBadge("main", BadgeVariantYellow, true)
	if !strings.Contains(got, capLeft) || !strings.Contains(got, capRight) {
		t.Fatalf("expected badge to render pill caps")
	}
	stripped := ansi.Strip(got)
	if !strings.Contains(stripped, " main ") {
		t.Fatalf("stripped badge = %q, want label body", stripped)
	}
}
