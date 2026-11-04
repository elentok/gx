package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadSkillsConfigDefaultsWhenBlockAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "gx-implement" {
		t.Fatalf("Skills.Implement = %q, want gx-implement", cfg.Skills.Implement)
	}
	if !slices.Equal(cfg.Skills.CodeReview, []string{"gx-thermo-nuclear-code-quality-review", "gx-test-review"}) {
		t.Fatalf("Skills.CodeReview = %+v, want [gx-thermo-nuclear-code-quality-review gx-test-review]", cfg.Skills.CodeReview)
	}
}

func TestLoadSkillsConfigPreservesUnspecifiedDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"skills":{"implement":"my-implement-skill"}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "my-implement-skill" {
		t.Fatalf("Skills.Implement = %q, want my-implement-skill", cfg.Skills.Implement)
	}
	if !slices.Equal(cfg.Skills.CodeReview, []string{"gx-thermo-nuclear-code-quality-review", "gx-test-review"}) {
		t.Fatalf("Skills.CodeReview = %+v, want default [gx-thermo-nuclear-code-quality-review gx-test-review]", cfg.Skills.CodeReview)
	}
}

func TestLoadSkillsConfigFullySpecified(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"skills":{"implement":"custom-implement","code-review":["review-a","review-b"]}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Skills.Implement != "custom-implement" {
		t.Fatalf("Skills.Implement = %q, want custom-implement", cfg.Skills.Implement)
	}
	if len(cfg.Skills.CodeReview) != 2 || cfg.Skills.CodeReview[0] != "review-a" || cfg.Skills.CodeReview[1] != "review-b" {
		t.Fatalf("Skills.CodeReview = %+v, want [review-a review-b]", cfg.Skills.CodeReview)
	}
}
