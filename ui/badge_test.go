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

func TestRenderBadgeGroupEmptyReturnsEmptyString(t *testing.T) {
	if got := RenderBadgeGroup(nil, false); got != "" {
		t.Fatalf("RenderBadgeGroup(nil) = %q, want empty", got)
	}
}

func TestRenderBadgeGroupSingleItemMatchesSingleBadge(t *testing.T) {
	group := RenderBadgeGroup([]BadgeGroupItem{{Label: "main", Fg: ColorGreen}}, false)
	single := RenderBadgeWithColor("main", ColorGreen, false, false)
	if group != single {
		t.Fatalf("single-item group = %q, want it to match a single badge %q", group, single)
	}
}

func TestRenderBadgeGroupJoinsNamesWithSharedBackground(t *testing.T) {
	got := RenderBadgeGroup([]BadgeGroupItem{
		{Label: "main", Fg: ColorGreen},
		{Label: "origin/main", Fg: ColorBlue},
	}, false)
	stripped := ansi.Strip(got)
	if stripped != "main origin/main" {
		t.Fatalf("stripped group = %q, want 'main origin/main'", stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI styling in badge group output")
	}
}

func TestRenderBadgeGroupUsesPillCapsWhenNerd(t *testing.T) {
	got := RenderBadgeGroup([]BadgeGroupItem{{Label: "main", Fg: ColorGreen}}, true)
	if !strings.Contains(got, capLeft) || !strings.Contains(got, capRight) {
		t.Fatalf("expected badge group to render pill caps when nerd fonts enabled")
	}
}
