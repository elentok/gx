package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
)

// runAgentModelCmd runs `gx agent model tier` with cfg as the loaded config.
func runAgentModelCmd(t *testing.T, cfg config.Config, env map[string]string, cwd, tier string) string {
	t.Helper()
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, env)
	d.getwd = func() (string, error) { return cwd, nil }
	d.loadConfig = func() (config.Config, error) { return cfg, nil }
	if err := execute([]string{"agent", "model", tier}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return stdout.String()
}

func TestAgentModel_ClaudeDefaults(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	for tier, want := range map[string]string{"light": "haiku\n", "worker": "sonnet\n", "strongest": "opus\n"} {
		if got := runAgentModelCmd(t, cfg, nil, t.TempDir(), tier); got != want {
			t.Errorf("tier %s = %q, want %q", tier, got, want)
		}
	}
}

func TestAgentModel_ConfiguredValueIsPrintedAsWritten(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Agents.Claude.Strongest.Model = "not-a-real-model"
	if got := runAgentModelCmd(t, cfg, nil, t.TempDir(), "strongest"); got != "not-a-real-model\n" {
		t.Errorf("got %q", got)
	}
}

func TestAgentModel_GXAgentCodexPrintsCodexSlug(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	got := runAgentModelCmd(t, cfg, map[string]string{"GX_AGENT": "codex"}, t.TempDir(), "light")
	if want := cfg.Agents.Codex.Light.Model + "\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAgentModel_EmptyModelPrintsNothing(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Agents.Claude.Light.Model = ""
	if got := runAgentModelCmd(t, cfg, nil, t.TempDir(), "light"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestAgentModel_ProjectOverlayApplies(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	store := t.TempDir()
	projectDir := filepath.Join(store, "proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pj := `{"name": "proj", "repo": "` + repo + `", "agents": {"claude": {"light": {"model": "project-light"}}}}`
	if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(pj), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.TicketStore.Path = store

	if got := runAgentModelCmd(t, cfg, nil, repo, "light"); got != "project-light\n" {
		t.Errorf("light = %q, want the project's override", got)
	}
	if got := runAgentModelCmd(t, cfg, nil, repo, "worker"); got != "sonnet\n" {
		t.Errorf("worker = %q, want the global value", got)
	}
}

func TestAgentModel_UnknownTierFails(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, nil)
	d.loadConfig = func() (config.Config, error) { return config.Default(), nil }
	if err := execute([]string{"agent", "model", "huge"}, d); err == nil {
		t.Error("want an error for an unknown tier")
	}
}
