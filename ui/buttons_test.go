package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderButtonPlain(t *testing.T) {
	got := RenderButton("Yes", false, false)
	if strings.Contains(got, capLeft) || strings.Contains(got, capRight) {
		t.Fatalf("plain button should not render pill caps")
	}
	if stripped := ansi.Strip(got); stripped != " Yes " {
		t.Fatalf("stripped button = %q, want %q", stripped, " Yes ")
	}
}

func TestRenderButtonNerdUsesPillCaps(t *testing.T) {
	got := RenderButton("Yes", true, true)
	if !strings.Contains(got, capLeft) || !strings.Contains(got, capRight) {
		t.Fatalf("nerd button should render pill caps")
	}
	if stripped := ansi.Strip(got); !strings.Contains(stripped, " Yes ") {
		t.Fatalf("stripped button = %q, want label body", stripped)
	}
}
