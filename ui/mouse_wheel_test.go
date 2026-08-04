package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWheelDirection(t *testing.T) {
	tests := []struct {
		name    string
		button  tea.MouseButton
		wantDir int
		wantOK  bool
	}{
		{"down", tea.MouseWheelDown, 1, true},
		{"up", tea.MouseWheelUp, -1, true},
		{"left is not a wheel button", tea.MouseLeft, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, ok := WheelDirection(tea.MouseWheelMsg{Button: tt.button})
			if dir != tt.wantDir || ok != tt.wantOK {
				t.Errorf("WheelDirection(%v) = (%d, %v), want (%d, %v)", tt.button, dir, ok, tt.wantDir, tt.wantOK)
			}
		})
	}
}

func TestClampScrollOffset(t *testing.T) {
	tests := []struct {
		name              string
		offset, total, vp int
		want              int
	}{
		{"negative offset clamps to 0", -5, 100, 10, 0},
		{"offset within range is unchanged", 20, 100, 10, 20},
		{"offset past max clamps to max", 95, 100, 10, 90},
		{"content fits viewport clamps to 0", 5, 8, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampScrollOffset(tt.offset, tt.total, tt.vp)
			if got != tt.want {
				t.Errorf("ClampScrollOffset(%d, %d, %d) = %d, want %d", tt.offset, tt.total, tt.vp, got, tt.want)
			}
		})
	}
}
