package ui

import "testing"

func TestRectContains(t *testing.T) {
	r := Rect{X: 2, Y: 3, W: 4, H: 5}
	tests := []struct {
		name string
		x, y int
		want bool
	}{
		{"inside", 3, 4, true},
		{"top-left corner is inclusive", 2, 3, true},
		{"bottom-right corner is exclusive", 6, 8, false},
		{"just inside bottom-right", 5, 7, true},
		{"left of rect", 1, 4, false},
		{"above rect", 3, 2, false},
		{"right of rect", 6, 4, false},
		{"below rect", 3, 8, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Contains(tt.x, tt.y); got != tt.want {
				t.Errorf("Rect(%+v).Contains(%d, %d) = %v, want %v", r, tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestHoverHitTest(t *testing.T) {
	left := Rect{X: 0, Y: 0, W: 5, H: 10}
	right := Rect{X: 6, Y: 0, W: 5, H: 10}

	tests := []struct {
		name      string
		x, y      int
		wantIndex int
		wantOK    bool
	}{
		{"matches first candidate", 2, 5, 0, true},
		{"matches second candidate", 8, 5, 1, true},
		{"gap between candidates matches nothing", 5, 5, -1, false},
		{"no candidates matches nothing", 100, 100, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := HoverHitTest(tt.x, tt.y, left, right)
			if index != tt.wantIndex || ok != tt.wantOK {
				t.Errorf("HoverHitTest(%d, %d) = (%d, %v), want (%d, %v)", tt.x, tt.y, index, ok, tt.wantIndex, tt.wantOK)
			}
		})
	}

	if index, ok := HoverHitTest(1, 1); ok || index != -1 {
		t.Errorf("HoverHitTest with no rects = (%d, %v), want (-1, false)", index, ok)
	}
}
