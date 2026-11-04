package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseAgentsSessionKeysDefaultWhenAbsent(t *testing.T) {
	cfg, err := Parse([]byte(`{"agents":{"claude":{"model":"opus"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Agents
	if !slices.Equal(a.LeaderKey, []string{"ctrl+a"}) {
		t.Errorf("LeaderKey = %v, want [ctrl+a]", a.LeaderKey)
	}
	if !a.ResumeOnStart || !a.Banners.Enabled || !a.Sounds.Enabled {
		t.Errorf("ResumeOnStart/Banners/Sounds = %v/%v/%v, want all true", a.ResumeOnStart, a.Banners.Enabled, a.Sounds.Enabled)
	}
	if a.Sounds.Done != "" || a.Sounds.Attention != "" || a.ClaudeArgs != nil {
		t.Errorf("Sounds/ClaudeArgs = %+v/%v, want built-in sounds and no args", a.Sounds, a.ClaudeArgs)
	}
}

func TestParseAgentsDefaultHarness(t *testing.T) {
	cfg, err := Parse([]byte(`{}`))
	if err != nil || cfg.Agents.DefaultHarness != "claude" {
		t.Fatalf("unset = %q, %v; want claude", cfg.Agents.DefaultHarness, err)
	}
	cfg, err = Parse([]byte(`{"agents":{"default-harness":"codex"}}`))
	if err != nil || cfg.Agents.DefaultHarness != "codex" {
		t.Fatalf("codex = %q, %v", cfg.Agents.DefaultHarness, err)
	}
	if _, err := Parse([]byte(`{"agents":{"default-harness":"gemini"}}`)); err == nil || !strings.Contains(err.Error(), "agents.default-harness") {
		t.Fatalf("bad value err = %v, want one naming the key", err)
	}
}

func TestParseAgentsResumeOnStartAlias(t *testing.T) {
	cfg, err := Parse([]byte(`{"agents":{"resume-on-start":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents.ResumeOnStart {
		t.Error("old resume-on-start key = false was ignored")
	}
	cfg, err = Parse([]byte(`{"agents":{"resume-on-start":false,"restore-sessions-on-start":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Agents.ResumeOnStart {
		t.Error("new key should win over the old alias")
	}
}

func TestParseAgentsSessionKeysSetValues(t *testing.T) {
	cfg, err := Parse([]byte(`{"agents":{
		"leader-key":"ctrl+b",
		"restore-sessions-on-start":false,
		"banners":{"enabled":false},
		"sounds":{"enabled":false,"done":"sounds/done.aiff","attention":"/abs/attention.aiff"},
		"claude-args":["--verbose","--add-dir","/tmp"]
	}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Agents
	if !slices.Equal(a.LeaderKey, []string{"ctrl+b"}) {
		t.Errorf("LeaderKey = %v, want [ctrl+b]", a.LeaderKey)
	}
	if a.ResumeOnStart || a.Banners.Enabled || a.Sounds.Enabled {
		t.Errorf("ResumeOnStart/Banners/Sounds = %v/%v/%v, want all false", a.ResumeOnStart, a.Banners.Enabled, a.Sounds.Enabled)
	}
	if a.Sounds.Done != "sounds/done.aiff" || a.Sounds.Attention != "/abs/attention.aiff" {
		t.Errorf("Sounds = %+v", a.Sounds)
	}
	if !slices.Equal(a.ClaudeArgs, []string{"--verbose", "--add-dir", "/tmp"}) {
		t.Errorf("ClaudeArgs = %v", a.ClaudeArgs)
	}
	if a.Claude.Model != defaultClaudeModel {
		t.Errorf("Claude.Model = %q, want default kept", a.Claude.Model)
	}
}

func TestParseAgentsFocusSidebarKey(t *testing.T) {
	cases := []struct {
		name, agents string
		want         []string
	}{
		{"default", `{}`, []string{"ctrl+q"}},
		{"string", `{"focus-sidebar-key":"ctrl+g"}`, []string{"ctrl+g"}},
		{"list", `{"focus-sidebar-key":["ctrl+g","f12"]}`, []string{"ctrl+g", "f12"}},
		{"blank falls back", `{"focus-sidebar-key":""}`, []string{"ctrl+q"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(`{"agents":` + tc.agents + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cfg.Agents.FocusSidebarKey, tc.want) {
				t.Errorf("FocusSidebarKey = %v, want %v", cfg.Agents.FocusSidebarKey, tc.want)
			}
		})
	}
}

func TestParseAgentsLeaderKey(t *testing.T) {
	cases := []struct {
		name, raw string
		want      []string
	}{
		{"string", `"ctrl+space"`, []string{"ctrl+space"}},
		{"list", `["ctrl+a","ctrl+]"]`, []string{"ctrl+a", "ctrl+]"}},
		{"number falls back", `3`, []string{"ctrl+a"}},
		{"empty string falls back", `""`, []string{"ctrl+a"}},
		{"empty list falls back", `[]`, []string{"ctrl+a"}},
		{"blank entry falls back", `["ctrl+b"," "]`, []string{"ctrl+a"}},
		{"mixed list falls back", `["ctrl+b",1]`, []string{"ctrl+a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse([]byte(`{"agents":{"leader-key":` + tc.raw + `}}`))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cfg.Agents.LeaderKey, tc.want) {
				t.Errorf("LeaderKey = %v, want %v", cfg.Agents.LeaderKey, tc.want)
			}
		})
	}
}

func TestDefaultAgentsConfig(t *testing.T) {
	cfg := DefaultAgentsConfig()
	want := map[string]struct {
		got  AgentConfig
		want AgentConfig
	}{
		"claude": {cfg.Claude, AgentConfig{
			Model: "sonnet", Effort: "medium",
			Light:     TierConfig{Model: "haiku"},
			Strongest: TierConfig{Model: "opus", Effort: "high"},
		}},
		"codex": {cfg.Codex, AgentConfig{
			Model: "gpt-6.1-sol", Effort: "medium",
			Light:     TierConfig{Model: "gpt-6-luna", Effort: "medium"},
			Strongest: TierConfig{Model: "gpt-6-astra", Effort: "high"},
		}},
	}
	for name, c := range want {
		if c.got != c.want {
			t.Errorf("%s = %+v, want %+v", name, c.got, c.want)
		}
	}
}

func TestLoadAgentsConfigTiers(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{"agents":{"claude":{"strongest":{"model":" fable ","effort":"max"},"light":{"effort":""}}}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Agents.Claude.Strongest, (TierConfig{Model: "fable", Effort: "max"}); got != want {
		t.Errorf("Strongest = %+v, want %+v", got, want)
	}
	if got, want := cfg.Agents.Claude.Light, (TierConfig{Model: "haiku"}); got != want {
		t.Errorf("Light = %+v, want untouched default %+v", got, want)
	}
	if cfg.Agents.Claude.Model != "sonnet" {
		t.Errorf("worker Model = %q, want default sonnet", cfg.Agents.Claude.Model)
	}
}

func TestLoadAgentsConfigDefaultsWhenBlockAbsent(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "sonnet" || cfg.Agents.Claude.Effort != "medium" {
		t.Fatalf("Agents.Claude = %+v, want sonnet/medium", cfg.Agents.Claude)
	}
	if cfg.Agents.Codex.Model != "gpt-6.1-sol" || cfg.Agents.Codex.Effort != "medium" {
		t.Fatalf("Agents.Codex = %+v, want gpt-6.1-sol/medium", cfg.Agents.Codex)
	}
}

func TestLoadAgentsConfigPartialFillsMissingKeysFromDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"claude":{"model":"opus"}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "opus" {
		t.Fatalf("Agents.Claude.Model = %q, want opus", cfg.Agents.Claude.Model)
	}
	if cfg.Agents.Claude.Effort != "medium" {
		t.Fatalf("Agents.Claude.Effort = %q, want default medium", cfg.Agents.Claude.Effort)
	}
	if cfg.Agents.Codex.Model != "gpt-6.1-sol" || cfg.Agents.Codex.Effort != "medium" {
		t.Fatalf("Agents.Codex = %+v, want untouched defaults", cfg.Agents.Codex)
	}
}

func TestLoadAgentsConfigExplicitEmptyStringMeansInherit(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"claude":{"model":"","effort":""}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Claude.Model != "" {
		t.Fatalf("Agents.Claude.Model = %q, want empty (inherit)", cfg.Agents.Claude.Model)
	}
	if cfg.Agents.Claude.Effort != "" {
		t.Fatalf("Agents.Claude.Effort = %q, want empty (inherit)", cfg.Agents.Claude.Effort)
	}
}

func TestLoadAgentsConfigTrimsWhitespace(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	body := `{"agents":{"codex":{"model":"  gpt-5.6-sol  ","effort":" high "}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents.Codex.Model != "gpt-5.6-sol" {
		t.Fatalf("Agents.Codex.Model = %q, want trimmed gpt-5.6-sol", cfg.Agents.Codex.Model)
	}
	if cfg.Agents.Codex.Effort != "high" {
		t.Fatalf("Agents.Codex.Effort = %q, want trimmed high", cfg.Agents.Codex.Effort)
	}
}

func TestParseRunnerWait(t *testing.T) {
	for name, tc := range map[string]struct {
		json string
		want time.Duration
	}{
		"default":    {`{}`, 5 * time.Second},
		"overridden": {`{"runner-wait":"12s"}`, 12 * time.Second},
		"unparsable": {`{"runner-wait":"soon"}`, 5 * time.Second},
		"zero":       {`{"runner-wait":"0s"}`, 5 * time.Second},
		"negative":   {`{"runner-wait":"-3s"}`, 5 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.json))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if cfg.RunnerWait != tc.want {
				t.Fatalf("RunnerWait = %s, want %s", cfg.RunnerWait, tc.want)
			}
		})
	}
}
