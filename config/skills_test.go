package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func loadFromBody(t *testing.T, body string) Config {
	t.Helper()
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })
	if body != "" {
		dir := filepath.Join(tmp, "gx")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestLoadPRFlowDefaults(t *testing.T) {
	cfg := loadFromBody(t, "")
	if cfg.PR.CIFixAttemptsPerCheck != 3 || cfg.PR.CIFixAttemptsPerPR != 10 || cfg.PR.NoChecksGrace != 2*time.Minute {
		t.Fatalf("PR = %+v", cfg.PR)
	}
	if len(cfg.Skills.CIFix) != 0 || cfg.Skills.PRText != "" {
		t.Fatalf("skills = %+v", cfg.Skills)
	}
}

func TestLoadPRFlowCustomValues(t *testing.T) {
	cfg := loadFromBody(t, `{"pr":{"ci-fix-attempts-per-check":5,"ci-fix-attempts-per-pr":20,"no-checks-grace":"0s"},
		"skills":{"ci-fix":["a","b"],"pr-text":"mine"}}`)
	if cfg.PR.CIFixAttemptsPerCheck != 5 || cfg.PR.CIFixAttemptsPerPR != 20 || cfg.PR.NoChecksGrace != 0 {
		t.Fatalf("PR = %+v", cfg.PR)
	}
	if !slices.Equal(cfg.Skills.CIFix, []string{"a", "b"}) || cfg.Skills.PRText != "mine" {
		t.Fatalf("skills = %+v", cfg.Skills)
	}
}

func TestLoadPRFlowIgnoresInvalidValues(t *testing.T) {
	cfg := loadFromBody(t, `{"pr":{"ci-fix-attempts-per-check":0,"ci-fix-attempts-per-pr":-1,"no-checks-grace":"-1m"}}`)
	if cfg.PR != DefaultPRConfig() {
		t.Fatalf("PR = %+v, want defaults", cfg.PR)
	}
}

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
