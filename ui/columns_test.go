package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderFixedColumnsUsesExactWidths(t *testing.T) {
	out := RenderFixedColumns([]FixedColumn{
		{Text: "abc", Width: 5},
		{Text: "xyz", Width: 4},
	})
	if got := ansi.StringWidth(out); got != 9 {
		t.Fatalf("width = %d, want 9: %q", got, out)
	}
}

func TestRenderFixedColumnsTruncatesAndKeepsAnsiSafe(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("abcdef")
	out := RenderFixedColumns([]FixedColumn{
		{Text: styled, Width: 4},
	})
	if got := ansi.StringWidth(out); got != 4 {
		t.Fatalf("width = %d, want 4: %q", got, out)
	}
	if !strings.Contains(ansi.Strip(out), "abc") {
		t.Fatalf("expected visible prefix in %q", out)
	}
}
