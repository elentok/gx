package ralphloop

import (
	"slices"
	"testing"

	"github.com/elentok/gx/config"
)

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
			args := agentArgs(tc.agent, "scratch", "epic", model, effort)
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
		args := agentArgs(AgentClaude, "scratch", "epic", model, effort)
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
