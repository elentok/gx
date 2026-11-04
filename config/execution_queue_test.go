package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadExecutionQueueConfigPreservesUnspecifiedDefault(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"execution-queue":{"max-agents-per-epic":4}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 4 {
		t.Fatalf("MaxConcurrentTicketsPerEpic = %d, want 4", cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic)
	}
	if cfg.ExecutionQueue.MaxConcurrentEpics != 2 {
		t.Fatalf("MaxConcurrentEpics = %d, want default 2", cfg.ExecutionQueue.MaxConcurrentEpics)
	}
}

func TestLoadExecutionQueueConfigClampsLimitsToOne(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"execution-queue":{"max-agents-per-epic":0,"max-agents":-3}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 1 || cfg.ExecutionQueue.MaxAgents != 1 {
		t.Fatalf("ExecutionQueue = %+v, want both limits clamped to 1", cfg.ExecutionQueue)
	}
}

func TestLoadExecutionQueueSpinKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		json       string
		wantCycles int
		wantWindow time.Duration
	}{
		"defaults":   {`{}`, 3, 5 * time.Minute},
		"overridden": {`{"execution-queue":{"spin-cycles":5,"spin-window":"90s"}}`, 5, 90 * time.Second},
		"bad window": {`{"execution-queue":{"spin-window":"soon"}}`, 3, 5 * time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			prev := userConfigDirFn
			userConfigDirFn = func() (string, error) { return tmp, nil }
			t.Cleanup(func() { userConfigDirFn = prev })
			dir := filepath.Join(tmp, "gx")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.json), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.ExecutionQueue.SpinCycles != tc.wantCycles || cfg.ExecutionQueue.SpinWindow != tc.wantWindow {
				t.Fatalf("spin = %d/%s, want %d/%s", cfg.ExecutionQueue.SpinCycles, cfg.ExecutionQueue.SpinWindow, tc.wantCycles, tc.wantWindow)
			}
		})
	}
}
