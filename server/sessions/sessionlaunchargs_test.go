package sessions

import (
	"slices"
	"testing"

	"github.com/elentok/gx/harness"
)

func TestLaunchArgs(t *testing.T) {
	t.Parallel()
	claudeArgs := []string{"--verbose"}
	roots := []string{"/repo/.git/worktrees/wt", "/repo/.git", "/store/proj"}
	for _, tc := range []struct {
		name       string
		meta       Meta
		claudeArgs []string
		want       []string
	}{
		{"claude with a remote-control name", Meta{Harness: "claude", RemoteControl: "proj:loop/01"}, claudeArgs, []string{"--verbose", "--remote-control", "proj:loop/01"}},
		{"claude without a name", Meta{Harness: "claude"}, claudeArgs, []string{"--verbose"}},
		{"claude design bans AskUserQuestion", Meta{Harness: "claude", Design: true}, claudeArgs, []string{"--verbose", "--disallowedTools", "AskUserQuestion"}},
		{"codex without a model or effort gets the shape only", Meta{Harness: "codex", RemoteControl: "proj:loop/01"}, claudeArgs, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots})},
		{"other harnesses get none", Meta{Harness: "agentfake"}, claudeArgs, nil},
		{"claude light", Meta{Harness: "claude", Tier: "light", Model: "haiku"}, claudeArgs, []string{"--verbose", "--model", "haiku"}},
		{"claude worker", Meta{Harness: "claude", Tier: "worker", Model: "sonnet", Effort: "medium"}, claudeArgs, []string{"--verbose", "--model", "sonnet", "--effort", "medium"}},
		{"claude strongest", Meta{Harness: "claude", Tier: "strongest", Model: "opus", Effort: "high"}, claudeArgs, []string{"--verbose", "--model", "opus", "--effort", "high"}},
		{"claude empty model", Meta{Harness: "claude", Tier: "worker", Effort: "high"}, nil, []string{"--effort", "high"}},
		{"claude empty effort", Meta{Harness: "claude", Tier: "worker", Model: "opus"}, nil, []string{"--model", "opus"}},
		{"claude-args hold --model", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x"}, []string{"--model", "x", "--effort", "high"}},
		{"claude-args hold --model=", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model=x"}, []string{"--model=x", "--effort", "high"}},
		{"claude-args hold --effort", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--effort", "low"}, []string{"--effort", "low", "--model", "opus"}},
		{"claude-args hold both", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x", "--effort", "low"}, []string{"--model", "x", "--effort", "low"}},
		{"claude resume with no saved tier", Meta{Harness: "claude"}, nil, nil},
		{"claude tier with remote control and design", Meta{Harness: "claude", Design: true, Model: "opus", RemoteControl: "p:l/1"}, nil,
			[]string{"--disallowedTools", "AskUserQuestion", "--model", "opus", "--remote-control", "p:l/1"}},
		{"codex light", Meta{Harness: "codex", Tier: "light", Model: "gpt-6-luna", Effort: "medium"}, claudeArgs, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6-luna", Effort: "medium", WritableRoots: roots})},
		{"codex worker", Meta{Harness: "codex", Tier: "worker", Model: "gpt-6.1-sol", Effort: "medium"}, nil, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6.1-sol", Effort: "medium", WritableRoots: roots})},
		{"codex strongest", Meta{Harness: "codex", Tier: "strongest", Model: "gpt-6-astra", Effort: "high"}, nil, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6-astra", Effort: "high", WritableRoots: roots})},
		{"codex empty model", Meta{Harness: "codex", Effort: "high"}, nil, harness.CodexArgs(harness.CodexArgsInput{Effort: "high", WritableRoots: roots})},
		{"codex empty effort", Meta{Harness: "codex", Model: "gpt-6-astra"}, nil, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6-astra", WritableRoots: roots})},
	} {
		if got := launchArgs(&tc.meta, tc.claudeArgs, roots); !slices.Equal(got, tc.want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !slices.Equal(claudeArgs, []string{"--verbose"}) {
		t.Errorf("configured claude args mutated: %v", claudeArgs)
	}
}
