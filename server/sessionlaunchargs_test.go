package server

import (
	"slices"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	t.Parallel()
	claudeArgs := []string{"--verbose"}
	for _, tc := range []struct {
		name       string
		meta       sessionMeta
		claudeArgs []string
		want       []string
	}{
		{"claude with a remote-control name", sessionMeta{Harness: "claude", RemoteControl: "proj:loop/01"}, claudeArgs, []string{"--verbose", "--remote-control", "proj:loop/01"}},
		{"claude without a name", sessionMeta{Harness: "claude"}, claudeArgs, []string{"--verbose"}},
		{"claude design bans AskUserQuestion", sessionMeta{Harness: "claude", Design: true}, claudeArgs, []string{"--verbose", "--disallowedTools", "AskUserQuestion"}},
		{"codex without a model or effort gets none", sessionMeta{Harness: "codex", RemoteControl: "proj:loop/01"}, claudeArgs, nil},
		{"claude light", sessionMeta{Harness: "claude", Tier: "light", Model: "haiku"}, claudeArgs, []string{"--verbose", "--model", "haiku"}},
		{"claude worker", sessionMeta{Harness: "claude", Tier: "worker", Model: "sonnet", Effort: "medium"}, claudeArgs, []string{"--verbose", "--model", "sonnet", "--effort", "medium"}},
		{"claude strongest", sessionMeta{Harness: "claude", Tier: "strongest", Model: "opus", Effort: "high"}, claudeArgs, []string{"--verbose", "--model", "opus", "--effort", "high"}},
		{"claude empty model", sessionMeta{Harness: "claude", Tier: "worker", Effort: "high"}, nil, []string{"--effort", "high"}},
		{"claude empty effort", sessionMeta{Harness: "claude", Tier: "worker", Model: "opus"}, nil, []string{"--model", "opus"}},
		{"claude-args hold --model", sessionMeta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x"}, []string{"--model", "x", "--effort", "high"}},
		{"claude-args hold --model=", sessionMeta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model=x"}, []string{"--model=x", "--effort", "high"}},
		{"claude-args hold --effort", sessionMeta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--effort", "low"}, []string{"--effort", "low", "--model", "opus"}},
		{"claude-args hold both", sessionMeta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x", "--effort", "low"}, []string{"--model", "x", "--effort", "low"}},
		{"claude resume with no saved tier", sessionMeta{Harness: "claude"}, nil, nil},
		{"claude tier with remote control and design", sessionMeta{Harness: "claude", Design: true, Model: "opus", RemoteControl: "p:l/1"}, nil,
			[]string{"--disallowedTools", "AskUserQuestion", "--model", "opus", "--remote-control", "p:l/1"}},
		{"codex light", sessionMeta{Harness: "codex", Tier: "light", Model: "gpt-6-luna", Effort: "medium"}, claudeArgs, []string{"--model", "gpt-6-luna", "-c", `model_reasoning_effort="medium"`}},
		{"codex worker", sessionMeta{Harness: "codex", Tier: "worker", Model: "gpt-6.1-sol", Effort: "medium"}, nil, []string{"--model", "gpt-6.1-sol", "-c", `model_reasoning_effort="medium"`}},
		{"codex strongest", sessionMeta{Harness: "codex", Tier: "strongest", Model: "gpt-6-astra", Effort: "high"}, nil, []string{"--model", "gpt-6-astra", "-c", `model_reasoning_effort="high"`}},
		{"codex empty model", sessionMeta{Harness: "codex", Effort: "high"}, nil, []string{"-c", `model_reasoning_effort="high"`}},
		{"codex empty effort", sessionMeta{Harness: "codex", Model: "gpt-6-astra"}, nil, []string{"--model", "gpt-6-astra"}},
	} {
		if got := launchArgs(&tc.meta, tc.claudeArgs); !slices.Equal(got, tc.want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !slices.Equal(claudeArgs, []string{"--verbose"}) {
		t.Errorf("configured claude args mutated: %v", claudeArgs)
	}
}
