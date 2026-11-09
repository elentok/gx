package webui

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestManifest(t *testing.T) {
	raw, err := os.ReadFile("public/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name            string `json:"name"`
		ShortName       string `json:"short_name"`
		ID              string `json:"id"`
		StartURL        string `json:"start_url"`
		Scope           string `json:"scope"`
		Display         string `json:"display"`
		ThemeColor      string `json:"theme_color"`
		BackgroundColor string `json:"background_color"`
		Icons           []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "gx" || m.ShortName != "gx" {
		t.Errorf("name/short_name = %q/%q, want gx", m.Name, m.ShortName)
	}
	if m.ID != "/" || m.StartURL != "/" || m.Scope != "/" {
		t.Errorf("id/start_url/scope = %q/%q/%q, want /", m.ID, m.StartURL, m.Scope)
	}
	if m.Display != "standalone" || m.ThemeColor != "#0a0d11" || m.BackgroundColor != "#0a0d11" {
		t.Errorf("display/colours = %q %q %q", m.Display, m.ThemeColor, m.BackgroundColor)
	}
	maskable := 0
	for _, icon := range m.Icons {
		f, err := os.Open(filepath.Join("public", filepath.Base(icon.Src)))
		if err != nil {
			t.Errorf("icon %s: %v", icon.Src, err)
			continue
		}
		cfg, err := png.DecodeConfig(f)
		_ = f.Close()
		if err != nil {
			t.Errorf("icon %s: %v", icon.Src, err)
			continue
		}
		if got := fmt.Sprintf("%dx%d", cfg.Width, cfg.Height); got != icon.Sizes {
			t.Errorf("icon %s is %s, manifest says %s", icon.Src, got, icon.Sizes)
		}
		if icon.Purpose == "maskable" {
			maskable++
		}
	}
	if len(m.Icons) != 3 || maskable != 1 {
		t.Errorf("icons = %d, maskable = %d, want 3 icons and exactly 1 maskable", len(m.Icons), maskable)
	}
}
