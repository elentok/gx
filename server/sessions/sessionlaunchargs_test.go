package sessions

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/tickets"
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
		{"claude design bans AskUserQuestion", Meta{Harness: "claude", Flow: tickets.FlowDesign}, claudeArgs, []string{"--verbose", "--disallowedTools", "AskUserQuestion"}},
		{"claude triage bans AskUserQuestion", Meta{Harness: "claude", Flow: tickets.FlowTriage, FlowKey: "proj"}, claudeArgs, []string{"--verbose", "--disallowedTools", "AskUserQuestion"}},
		{"codex without a model or effort gets the shape only", Meta{Harness: "codex", RemoteControl: "proj:loop/01"}, claudeArgs, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots, QuestionTool: true})},
		{"other harnesses get none", Meta{Harness: "agentfake"}, claudeArgs, nil},
		{"claude model and effort", Meta{Harness: "claude", Model: "sonnet", Effort: "medium"}, claudeArgs, []string{"--verbose", "--model", "sonnet", "--effort", "medium"}},
		{"claude empty model", Meta{Harness: "claude", Tier: "worker", Effort: "high"}, nil, []string{"--effort", "high"}},
		{"claude empty effort", Meta{Harness: "claude", Tier: "worker", Model: "opus"}, nil, []string{"--model", "opus"}},
		{"claude-args hold --model", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x"}, []string{"--model", "x", "--effort", "high"}},
		{"claude-args hold --model=", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model=x"}, []string{"--model=x", "--effort", "high"}},
		{"claude-args hold --effort", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--effort", "low"}, []string{"--effort", "low", "--model", "opus"}},
		{"claude-args hold both", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x", "--effort", "low"}, []string{"--model", "x", "--effort", "low"}},
		{"claude tier with remote control and design", Meta{Harness: "claude", Flow: tickets.FlowDesign, Model: "opus", RemoteControl: "p:l/1"}, nil,
			[]string{"--disallowedTools", "AskUserQuestion", "--model", "opus", "--remote-control", "p:l/1"}},
		{"codex light", Meta{Harness: "codex", Tier: "light", Model: "gpt-6-luna", Effort: "medium"}, claudeArgs, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6-luna", Effort: "medium", WritableRoots: roots, QuestionTool: true})},
		{"codex empty model", Meta{Harness: "codex", Effort: "high"}, nil, harness.CodexArgs(harness.CodexArgsInput{Effort: "high", WritableRoots: roots, QuestionTool: true})},
		{"codex empty effort", Meta{Harness: "codex", Model: "gpt-6-astra"}, nil, harness.CodexArgs(harness.CodexArgsInput{Model: "gpt-6-astra", WritableRoots: roots, QuestionTool: true})},
		{"codex design keeps the guard", Meta{Harness: "codex", Flow: tickets.FlowDesign}, nil, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots})},
		{"codex triage keeps the guard", Meta{Harness: "codex", Flow: tickets.FlowTriage, FlowKey: "proj"}, nil, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots})},
		{"codex queue ticket keeps the guard", Meta{Harness: "codex", Ticket: "gx:e/01"}, nil, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots})},
		{"codex fix keeps the guard", Meta{Harness: "codex", FixOf: "gx:e/01"}, nil, harness.CodexArgs(harness.CodexArgsInput{WritableRoots: roots})},
	} {
		want := tc.want
		if tc.meta.Harness == "claude" && tc.meta.Flow == "" {
			want = append(slices.Clone(want), harness.ClaudeAskHookArgs()...)
		}
		if got := launchArgs(&tc.meta, tc.claudeArgs, roots); !slices.Equal(got, want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, want)
		}
	}
	if !slices.Equal(claudeArgs, []string{"--verbose"}) {
		t.Errorf("configured claude args mutated: %v", claudeArgs)
	}
}

// A plain Codex chat, new or resumed (both run launchArgs), turns the question
// tool on and drops its guard. Every other Codex launch (conflict and doctor
// agents among them run as a Queue ticket or a Fix session) keeps the guard
// and never gets the flag.
func TestLaunchArgs_CodexQuestionToolOnPlainChatOnly(t *testing.T) {
	t.Parallel()
	guard := func(args []string) bool {
		return slices.ContainsFunc(args, func(a string) bool {
			return strings.HasPrefix(a, "hooks.PreToolUse=") && strings.Contains(a, harness.CodexQuestionTool)
		})
	}
	for _, tc := range []struct {
		name  string
		meta  Meta
		plain bool
	}{
		{"plain chat", Meta{Harness: "codex"}, true},
		{"queue ticket", Meta{Harness: "codex", Ticket: "gx:e/01"}, false},
		{"fix agent", Meta{Harness: "codex", FixOf: "gx:e/01"}, false},
		{"design", Meta{Harness: "codex", Flow: tickets.FlowDesign}, false},
		{"triage", Meta{Harness: "codex", Flow: tickets.FlowTriage, FlowKey: "proj"}, false},
	} {
		args := launchArgs(&tc.meta, nil, []string{"/repo/.git"})
		if got := slices.Contains(args, harness.CodexQuestionToolFlag); got != tc.plain {
			t.Errorf("%s: question tool flag = %v, want %v", tc.name, got, tc.plain)
		}
		if got := slices.Contains(args, "--no-daemon"); got != tc.plain {
			t.Errorf("%s: --no-daemon = %v, want %v", tc.name, got, tc.plain)
		}
		if got := guard(args); got == tc.plain {
			t.Errorf("%s: question guard hook = %v, want %v", tc.name, got, !tc.plain)
		}
	}
}

// askHookInSettings reports whether the merged --settings of a claude launch
// carries a PreToolUse hook for AskUserQuestion with the 600 s timeout.
func askHookInSettings(t *testing.T, args []string) bool {
	t.Helper()
	merged := harness.MergeClaudeSettings(args)
	for i, a := range merged {
		if a != "--settings" || i+1 >= len(merged) {
			continue
		}
		var s struct {
			Hooks struct {
				PreToolUse []struct {
					Matcher string `json:"matcher"`
					Hooks   []struct {
						Command string `json:"command"`
						Timeout int    `json:"timeout"`
					} `json:"hooks"`
				} `json:"PreToolUse"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(merged[i+1]), &s); err != nil {
			t.Fatal(err)
		}
		for _, g := range s.Hooks.PreToolUse {
			if g.Matcher == "AskUserQuestion" && len(g.Hooks) == 1 && g.Hooks[0].Command == "gx agent ask-question" && g.Hooks[0].Timeout == 600 {
				return true
			}
		}
	}
	return false
}

func TestLaunchArgs_ClaudeAskHookOnPlainLaunchOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		meta Meta
		want bool
	}{
		{"plain", Meta{Harness: "claude"}, true},
		{"design", Meta{Harness: "claude", Flow: tickets.FlowDesign}, false},
		{"triage", Meta{Harness: "claude", Flow: tickets.FlowTriage, FlowKey: "proj"}, false},
		{"queue ticket", Meta{Harness: "claude", Ticket: "gx:e/01"}, false},
		{"fix", Meta{Harness: "claude", FixOf: "gx:e/01"}, false},
	} {
		if got := askHookInSettings(t, launchArgs(&tc.meta, []string{"--verbose"}, nil)); got != tc.want {
			t.Errorf("%s: ask hook installed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
