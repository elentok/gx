package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadWithConfig(t *testing.T, body string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDirFn = prev })
	if err := os.MkdirAll(filepath.Join(dir, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gx", "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load()
}

func TestLoad_EvolveDefaults(t *testing.T) {
	cfg, err := loadWithConfig(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Evolve != EvolveOff || cfg.EvolveDeepReadCap != 8 {
		t.Fatalf("got %q / %d", cfg.Evolve, cfg.EvolveDeepReadCap)
	}
}

func TestLoad_EvolveSet(t *testing.T) {
	cfg, err := loadWithConfig(t, `{"evolve":"contributor","evolve.deep-read-cap":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Evolve != EvolveContributor || cfg.EvolveDeepReadCap != 3 {
		t.Fatalf("got %q / %d", cfg.Evolve, cfg.EvolveDeepReadCap)
	}
}

func TestLoad_EvolveUnknownModeIsError(t *testing.T) {
	if _, err := loadWithConfig(t, `{"evolve":"maintaner"}`); err == nil {
		t.Fatal("want error for unknown evolve mode")
	}
}

func TestEvolveMode_ProjectOptOut(t *testing.T) {
	cfg := Default()
	cfg.Evolve = EvolveMaintainer
	no, yes := false, true
	if got := cfg.EvolveMode(ProjectFile{}); got != EvolveMaintainer {
		t.Fatalf("default project: %q", got)
	}
	if got := cfg.EvolveMode(ProjectFile{Evolve: &yes}); got != EvolveMaintainer {
		t.Fatalf("evolve true: %q", got)
	}
	if cfg.EvolveEnabled(ProjectFile{Evolve: &no}) {
		t.Fatal("evolve:false must disable")
	}
}
