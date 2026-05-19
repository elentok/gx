package config

import (
	"encoding/json"
	"testing"
)

func TestResolveY_Lines(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindLines, Lines: 3}
	// screenH=20, fgH=5 → 20-5-3 = 12
	if got := b.ResolveY(20, 5); got != 12 {
		t.Errorf("Lines: got %d, want 12", got)
	}
}

func TestResolveY_Percent(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindPercent, Percent: 10}
	// screenH=100, fgH=10, pad=10 → 100-10-10 = 80
	if got := b.ResolveY(100, 10); got != 80 {
		t.Errorf("Percent: got %d, want 80", got)
	}
}

func TestResolveY_Center(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindCenter}
	// (screenH - fgH) / 2 = (20-4)/2 = 8
	if got := b.ResolveY(20, 4); got != 8 {
		t.Errorf("Center: got %d, want 8", got)
	}
}

func TestResolveY_ClampsToZero(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindLines, Lines: 100}
	if got := b.ResolveY(5, 5); got != 0 {
		t.Errorf("Clamp: got %d, want 0", got)
	}
}

func TestMarshalJSON_Lines(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindLines, Lines: 5}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(data) != "5" {
		t.Errorf("got %s, want 5", data)
	}
}

func TestMarshalJSON_Percent(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindPercent, Percent: 15}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(data) != `"15%"` {
		t.Errorf("got %s, want \"15%%\"", data)
	}
}

func TestMarshalJSON_Center(t *testing.T) {
	b := InputModalBottom{Kind: InputModalBottomKindCenter}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(data) != `"center"` {
		t.Errorf("got %s, want \"center\"", data)
	}
}
