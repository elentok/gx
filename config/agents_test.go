package config

import (
	"slices"
	"strings"
	"testing"
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
