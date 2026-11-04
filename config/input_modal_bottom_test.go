package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestLoadInputModalBottomNumeric(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":10}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindLines || cfg.InputModalBottom.Lines != 10 {
		t.Fatalf("got %+v, want Lines=10", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomPercent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"20%"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindPercent || cfg.InputModalBottom.Percent != 20 {
		t.Fatalf("got %+v, want Percent=20", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomCenter(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"center"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.InputModalBottom.Kind != InputModalBottomKindCenter {
		t.Fatalf("got %+v, want Center", cfg.InputModalBottom)
	}
}

func TestLoadInputModalBottomMissingUsesDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := DefaultInputModalBottom()
	if cfg.InputModalBottom.Kind != d.Kind || cfg.InputModalBottom.Percent != d.Percent {
		t.Fatalf("got %+v, want default %+v", cfg.InputModalBottom, d)
	}
}

func TestLoadInputModalBottomInvalidFallsBackToDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"input-modal-bottom":"bogus"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := DefaultInputModalBottom()
	if cfg.InputModalBottom.Kind != d.Kind || cfg.InputModalBottom.Percent != d.Percent {
		t.Fatalf("got %+v, want default %+v", cfg.InputModalBottom, d)
	}
}
