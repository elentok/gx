package agent

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
)

func TestSkillPrompt_MatchesTheHarnessSkillCommand(t *testing.T) {
	t.Parallel()
	for _, agent := range []AgentKind{AgentClaude, AgentCodex, "fake"} {
		for _, addr := range []string{"", "gx:epic/01"} {
			want := harness.SkillCommand(string(agent), "gx-implement", addr)
			if got := SkillPrompt(agent, "gx-implement", addr); got != want {
				t.Errorf("SkillPrompt(%q, %q) = %q, want %q", agent, addr, got, want)
			}
		}
	}
	if got := SkillPrompt(AgentCodex, "gx-implement", "gx:epic/01"); got != "$gx-implement gx:epic/01" {
		t.Errorf("codex prompt = %q", got)
	}
}

// launchSettings is the one --settings claude gets: the guard hook, the lean
// context keys plus normal editor mode.
const launchSettings = `{"disableClaudeAiConnectors":true,"editorMode":"normal","hooks":{"PreToolUse":[{"hooks":[{"command":"gx agent guard-bash","type":"command"}],"matcher":"Bash"}]},"syncClaudeAiSkills":false}`

// notARepo is a cwd git cannot read, so AgentArgs adds no git roots.
const notARepo = "/nonexistent/not-a-repo"

var codexTestRoots = []string{"scratch", filepath.Join("scratch", "epic")}

func TestAgentArgs_ModelAndEffort(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		agent  AgentKind
		model  string
		effort string
		want   []string
	}{
		{
			name:  "claude both empty unchanged",
			agent: AgentClaude,
			want:  []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile()},
		},
		{
			name:   "claude both set",
			agent:  AgentClaude,
			model:  "sonnet",
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--model", "sonnet", "--effort", "medium"},
		},
		{
			name:  "claude model only",
			agent: AgentClaude,
			model: "sonnet",
			want:  []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--model", "sonnet"},
		},
		{
			name:   "claude effort only",
			agent:  AgentClaude,
			effort: "medium",
			want:   []string{"--permission-mode", "auto", "--settings", launchSettings, "--disallowedTools", loopDisallowedTools, "--append-system-prompt-file", harness.GXSystemPromptFile(), "--effort", "medium"},
		},
		{
			name:  "codex both empty unchanged",
			agent: AgentCodex,
			want:  harness.CodexArgs(harness.CodexArgsInput{WritableRoots: codexTestRoots}),
		},
		{
			name:   "codex both set",
			agent:  AgentCodex,
			model:  "gpt-5.6-sol",
			effort: "medium",
			want:   harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-5.6-sol", Effort: "medium", WritableRoots: codexTestRoots}),
		},
		{
			name:  "codex model only",
			agent: AgentCodex,
			model: "gpt-5.6-sol",
			want:  harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-5.6-sol", WritableRoots: codexTestRoots}),
		},
		{
			name:   "codex effort only",
			agent:  AgentCodex,
			effort: "medium",
			want:   harness.CodexArgs(harness.CodexArgsInput{Effort: "medium", WritableRoots: codexTestRoots}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := AgentArgs(tc.agent, notARepo, "scratch", "epic", tc.model, tc.effort)
			if !slices.Equal(got, tc.want) {
				t.Errorf("AgentArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveTier(t *testing.T) {
	agents := config.DefaultAgentsConfig()
	agents.Codex.Light.Effort = ""

	for _, tc := range []struct {
		name         string
		agent        AgentKind
		tier         Tier
		wantModel    string
		wantEffort   string
		wantFlagTail []string
	}{
		{"claude light has no effort", AgentClaude, TierLight, "haiku", "", []string{"--model", "haiku"}},
		{"claude worker", AgentClaude, TierWorker, "sonnet", "medium", []string{"--model", "sonnet", "--effort", "medium"}},
		{"claude strongest", AgentClaude, TierStrongest, "opus", "high", []string{"--model", "opus", "--effort", "high"}},
		{"codex light with empty effort", AgentCodex, TierLight, "gpt-6-luna", "", []string{"--model", "gpt-6-luna"}},
		{"codex worker", AgentCodex, TierWorker, "gpt-6.1-sol", "medium", []string{"--model", "gpt-6.1-sol", "-c", `model_reasoning_effort="medium"`}},
		{"codex strongest", AgentCodex, TierStrongest, "gpt-6-astra", "high", []string{"--model", "gpt-6-astra", "-c", `model_reasoning_effort="high"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, effort := ResolveTier(agents, tc.agent, tc.tier)
			if model != tc.wantModel || effort != tc.wantEffort {
				t.Fatalf("ResolveTier = %q, %q; want %q, %q", model, effort, tc.wantModel, tc.wantEffort)
			}
			args := AgentArgs(tc.agent, notARepo, "scratch", "epic", model, effort)
			if !slices.Equal(args[len(args)-len(tc.wantFlagTail):], tc.wantFlagTail) {
				t.Errorf("args = %v, want tail %v", args, tc.wantFlagTail)
			}
		})
	}

	t.Run("empty model and effort give no flags", func(t *testing.T) {
		empty := config.AgentsConfig{}
		model, effort := ResolveTier(empty, AgentClaude, TierStrongest)
		if model != "" || effort != "" {
			t.Fatalf("ResolveTier = %q, %q; want empty", model, effort)
		}
		args := AgentArgs(AgentClaude, notARepo, "scratch", "epic", model, effort)
		if slices.Contains(args, "--model") || slices.Contains(args, "--effort") {
			t.Errorf("args = %v, want no model or effort flag", args)
		}
	})
}

func TestAgentFor(t *testing.T) {
	for name, want := range map[string]AgentKind{"codex": AgentCodex, "claude": AgentClaude, "": AgentClaude} {
		if got := AgentFor(name); got != want {
			t.Errorf("AgentFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTierValid(t *testing.T) {
	if len(AllTiers) != 3 {
		t.Errorf("AllTiers = %v, want 3 tiers", AllTiers)
	}
	for _, tier := range AllTiers {
		if !tier.Valid() {
			t.Errorf("%q should be valid", tier)
		}
	}
	for _, tier := range []Tier{"", "bogus"} {
		if tier.Valid() {
			t.Errorf("%q should be invalid", tier)
		}
	}
}
