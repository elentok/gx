package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestProgressBar(t *testing.T) {
	plain := lipgloss.NewStyle()
	tests := []struct {
		name     string
		fraction float64
		cells    int
		want     string
	}{
		{"zero", 0, 7, "▱▱▱▱▱▱▱"},
		{"one", 1, 7, "▰▰▰▰▰▰▰"},
		{"just under a cell edge", 2.0/7 - 1e-6, 7, "▰▱▱▱▱▱▱"},
		{"just under one is never full", 1 - 1e-6, 7, "▰▰▰▰▰▰▱"},
		{"below zero clamps", -0.5, 7, "▱▱▱▱▱▱▱"},
		{"above one clamps", 1.5, 7, "▰▰▰▰▰▰▰"},
		{"7/7 exact", 7.0 / 7, 7, "▰▰▰▰▰▰▰"},
		{"3/3 exact", 3.0 / 3, 3, "▰▰▰"},
		{"1/3 of 7 floors", 1.0 / 3, 7, "▰▰▱▱▱▱▱"},
		{"zero cells", 0.5, 0, ""},
		{"negative cells", 0.5, -2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansi.Strip(ProgressBar(tt.fraction, tt.cells, plain, plain))
			if got != tt.want {
				t.Errorf("ProgressBar(%v, %d) = %q, want %q", tt.fraction, tt.cells, got, tt.want)
			}
		})
	}
}

func TestProgressBar_StylesLandOnTheirGlyphs(t *testing.T) {
	fill := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	rest := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	want := fill.Render("▰▰") + rest.Render("▱▱")
	if got := ProgressBar(0.5, 4, fill, rest); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
