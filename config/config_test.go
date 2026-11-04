package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultLogConfig(t *testing.T) {
	cfg := DefaultLogConfig()
	if len(cfg.ImportantRefs) == 0 {
		t.Error("expected non-empty ImportantRefs")
	}
	found := false
	for _, rule := range cfg.ImportantRefs {
		for _, p := range rule.Patterns {
			if p == "^main$" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected ^main$ pattern in DefaultLogConfig")
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
	if cfg.StageDiffContextLines != 1 {
		t.Fatalf("StageDiffContextLines = %d, want 1", cfg.StageDiffContextLines)
	}
	if cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic != 2 || cfg.ExecutionQueue.MaxConcurrentEpics != 2 {
		t.Fatalf("ExecutionQueue = %+v, want both limits to default to 2", cfg.ExecutionQueue)
	}
	if cfg.ExecutionQueue.MaxAgents != 4 {
		t.Fatalf("MaxAgents = %d, want default 4", cfg.ExecutionQueue.MaxAgents)
	}
	if cfg.AgentRunner != "auto" {
		t.Fatalf("AgentRunner = %q, want auto", cfg.AgentRunner)
	}
}

func TestLoadAgentRunner(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"agent-runner":"headless"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentRunner != "headless" {
		t.Fatalf("AgentRunner = %q, want headless", cfg.AgentRunner)
	}
}

// The old cap keys are treated like any other unknown key: no effect, no warning.
func TestLoadOldCapKeysAreUnknown(t *testing.T) {
	cases := []struct {
		name, json  string
		wantPerEpic int
	}{
		{"new key applies", `{"execution-queue":{"max-agents-per-epic":3}}`, 3},
		{"old per-epic key has no effect", `{"execution-queue":{"max-concurrent-tickets-per-epic":4}}`, 2},
		{"old per-epic key does not override the new one", `{"execution-queue":{"max-agents-per-epic":3,"max-concurrent-tickets-per-epic":4}}`, 3},
		{"max-concurrent-epics has no effect", `{"execution-queue":{"max-concurrent-epics":9}}`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			prev := userConfigDirFn
			userConfigDirFn = func() (string, error) { return tmp, nil }
			t.Cleanup(func() { userConfigDirFn = prev })
			if err := os.MkdirAll(filepath.Join(tmp, "gx"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tmp, "gx", "config.json"), []byte(tc.json), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic; got != tc.wantPerEpic {
				t.Errorf("per-epic = %d, want %d", got, tc.wantPerEpic)
			}
			if got := cfg.ExecutionQueue.MaxConcurrentEpics; got != 2 {
				t.Errorf("MaxConcurrentEpics = %d, want default 2", got)
			}
		})
	}
}

func TestLoadSubscriptionConfigDefaultsToUnsuppressed(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Subscription.SuppressExtraUsageWarning {
		t.Fatal("SuppressExtraUsageWarning = true, want false by default")
	}
}

func TestLoadSubscriptionConfigSuppressWarning(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"subscription":{"suppress-extra-usage-warning":true}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Subscription.SuppressExtraUsageWarning {
		t.Fatal("SuppressExtraUsageWarning = false, want true")
	}
}

func TestLoadRecoveryNotifyHold(t *testing.T) {
	for _, tc := range []struct {
		name, json string
		want       time.Duration
	}{
		{"absent", `{}`, DefaultRecoveryNotifyHold},
		{"set", `{"recovery":{"notify-hold":"90s"}}`, 90 * time.Second},
		{"invalid", `{"recovery":{"notify-hold":"soon"}}`, DefaultRecoveryNotifyHold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			prev := userConfigDirFn
			userConfigDirFn = func() (string, error) { return tmp, nil }
			t.Cleanup(func() { userConfigDirFn = prev })
			dir := filepath.Join(tmp, "gx")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.json), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Recovery.NotifyHold != tc.want {
				t.Fatalf("NotifyHold = %v, want %v", cfg.Recovery.NotifyHold, tc.want)
			}
		})
	}
}

func TestLoadMaxOneOffAgents(t *testing.T) {
	cases := []struct {
		name, json string
		want       int
	}{
		{"default", `{}`, 3},
		{"global value", `{"execution-queue":{"max-one-off-agents":5}}`, 5},
		{"non-positive is clamped", `{"execution-queue":{"max-one-off-agents":0}}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			prev := userConfigDirFn
			userConfigDirFn = func() (string, error) { return tmp, nil }
			t.Cleanup(func() { userConfigDirFn = prev })
			writeBudgetConfig(t, tmp, tc.json)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.ExecutionQueue.MaxOneOffAgents; got != tc.want {
				t.Fatalf("MaxOneOffAgents = %d, want %d", got, tc.want)
			}
		})
	}
}

func writeBudgetConfig(t *testing.T, tmp string, body string) {
	t.Helper()
	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadParsesUseNerdFontIcons(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"use-nerdfont-icons":true}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true")
	}
}

func TestLoadParsesStageDiffContextLines(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"stage-diff-context-lines":3}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StageDiffContextLines != 3 {
		t.Fatalf("StageDiffContextLines = %d, want 3", cfg.StageDiffContextLines)
	}
}

func TestLoadClampsStageDiffContextLines(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"stage-diff-context-lines":999}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StageDiffContextLines != 20 {
		t.Fatalf("StageDiffContextLines = %d, want 20", cfg.StageDiffContextLines)
	}
}

func TestInitCreatesDefaultConfig(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) == "" {
		t.Fatal("expected non-empty config file")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.UseNerdFontIcons {
		t.Fatal("UseNerdFontIcons = false, want true by default")
	}
	if cfg.StageDiffContextLines != 1 {
		t.Fatalf("StageDiffContextLines = %d, want 1 by default", cfg.StageDiffContextLines)
	}
}

func TestLoadNameAliases(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"name-aliases":{"my-project-frontend":"project-fe"}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.NameAliases["my-project-frontend"]; got != "project-fe" {
		t.Fatalf("NameAliases lookup = %q, want %q", got, "project-fe")
	}
}

func TestLoadLogHideRefsPreservesDefaultImportantRefs(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"log":{"hide-refs":["refs/remotes/origin/HEAD"]}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Log.HideRefs) != 1 || cfg.Log.HideRefs[0] != "refs/remotes/origin/HEAD" {
		t.Fatalf("HideRefs = %v, want [refs/remotes/origin/HEAD]", cfg.Log.HideRefs)
	}
	def := DefaultLogConfig()
	if len(cfg.Log.ImportantRefs) != len(def.ImportantRefs) {
		t.Fatalf("ImportantRefs len = %d, want %d (defaults preserved)", len(cfg.Log.ImportantRefs), len(def.ImportantRefs))
	}
}

func TestInitFailsIfConfigExists(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	if _, err := Init(); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(); err == nil {
		t.Fatal("expected error on second Init, got nil")
	}
}
