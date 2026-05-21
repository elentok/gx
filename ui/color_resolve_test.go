package ui

import (
	"testing"
)

func TestResolveColor_Named(t *testing.T) {
	names := []string{"blue", "green", "yellow", "orange", "mauve", "teal", "red", "surface"}
	for _, name := range names {
		c, err := ResolveColor(name)
		if err != nil {
			t.Errorf("ResolveColor(%q) error: %v", name, err)
		}
		if c == nil {
			t.Errorf("ResolveColor(%q) = nil", name)
		}
	}
}

func TestResolveColor_Hex6(t *testing.T) {
	c, err := ResolveColor("#89b4fa")
	if err != nil {
		t.Fatalf("ResolveColor hex6: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil color")
	}
}

func TestResolveColor_Hex3(t *testing.T) {
	c, err := ResolveColor("#f0f")
	if err != nil {
		t.Fatalf("ResolveColor hex3: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil color")
	}
}

func TestResolveColor_InvalidHex(t *testing.T) {
	_, err := ResolveColor("#gg0000")
	if err == nil {
		t.Error("expected error for invalid hex color")
	}

	_, err = ResolveColor("#12345")
	if err == nil {
		t.Error("expected error for wrong-length hex")
	}
}

func TestResolveColor_Unknown(t *testing.T) {
	_, err := ResolveColor("notacolor")
	if err == nil {
		t.Error("expected error for unknown color name")
	}
}
