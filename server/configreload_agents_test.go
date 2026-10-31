package server_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestSettingsFromConfig_CarriesAgentsSessionKeys(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Agents.ResumeOnStart = false
	cfg.Agents.Banners.Enabled = false
	cfg.Agents.Sounds = config.AgentsSoundsConfig{Enabled: false, Done: "done.aiff", Attention: "attention.aiff"}
	cfg.Agents.ClaudeArgs = []string{"--verbose"}

	s := server.SettingsFromConfig(cfg)
	if s.AgentsResumeOnStart || s.AgentsBanners || s.AgentsSounds != cfg.Agents.Sounds || !slices.Equal(s.AgentsClaudeArgs, cfg.Agents.ClaudeArgs) {
		t.Errorf("settings = resume %v, banners %v, sounds %+v, args %v; want the config's agents keys",
			s.AgentsResumeOnStart, s.AgentsBanners, s.AgentsSounds, s.AgentsClaudeArgs)
	}
}

// A reload reports each changed agents key, so the live settings took it.
func TestConfigReload_AgentsSessionKeysApply(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	servertest.WriteConfig(t, path, store, map[string]any{"agents": map[string]any{
		"restore-sessions-on-start": false,
		"banners":                   map[string]any{"enabled": false},
		"sounds":                    map[string]any{"enabled": false, "done": "done.aiff"},
		"claude-args":               []string{"--verbose"},
		"fix-agent-remote-control":  true,
		"default-harness":           "codex",
	}})
	res := reload(t, h)
	want := []string{"agents.restore-sessions-on-start", "agents.banners.enabled", "agents.sounds", "agents.claude-args", "agents.fix-agent-remote-control", "agents.default-harness"}
	for _, key := range want {
		if !slices.Contains(res.Changed, key) {
			t.Errorf("reload = %+v, want %s changed", res, key)
		}
	}
}
