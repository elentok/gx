package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/tickets"
)

// startCodexAndReadEnv starts a Codex holder through a stand-in that writes its
// environment to a FIFO, and returns the GX_CODEX_SWITCHED_AT value it saw.
func startCodexAndReadEnv(t *testing.T, meta *Meta, override *launchOverride) string {
	t.Helper()
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.fifo")
	if err := syscall.Mkfifo(envFile, 0o600); err != nil {
		t.Fatal(err)
	}
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	mg := &Manager{
		settings: set,
		projects: projects.NewRegistry(func() projects.Env { return projects.Env{} }, nil, nil, set, nil, nil),
		cfg:      SessionsConfig{Holder: []string{"/bin/sh", "-c", `env > "$1"`, "sh", envFile}},
	}
	meta.Harness = string(harness.Codex)
	meta.Dir = dir
	if err := mg.startSessionHolderAs(dir, meta, override, 80, 24, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, harness.CodexSwitchedAtEnv+"="); ok {
			return v
		}
	}
	return ""
}

// A switch launch carries its own time: the saved record still holds the
// previous switch until the new agent is idle.
func TestStartSessionHolderAs_CodexSwitchLaunchGetsTheNewSwitchTime(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	meta := &Meta{ID: "s", ConversationID: "c", SwitchedAt: old}
	got := startCodexAndReadEnv(t, meta, &launchOverride{Model: "m", SwitchedAt: now})
	if want := now.Format(time.RFC3339Nano); got != want {
		t.Errorf("%s = %q, want %q", harness.CodexSwitchedAtEnv, got, want)
	}
	if !meta.SwitchedAt.Equal(old) {
		t.Errorf("saved SwitchedAt = %v, want the old %v kept", meta.SwitchedAt, old)
	}
}

func TestStartSessionHolderAs_CodexPlainLaunchKeepsTheSavedSwitchTime(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := &Meta{ID: "s", ConversationID: "c", SwitchedAt: old}
	if got, want := startCodexAndReadEnv(t, meta, nil), old.Format(time.RFC3339Nano); got != want {
		t.Errorf("%s = %q, want %q", harness.CodexSwitchedAtEnv, got, want)
	}
}

const testFilesPrompt = "/cache/gx/files-prompt.md"

// Every kind of Claude session, new or resumed, carries the files-only flag
// and never the ask text; a Codex session's args carry no prompt flag (its
// launch option adds the sentence, tested in the harness package).
func TestLaunchArgs_FilesPromptReachesClaudeLaunchAndResume(t *testing.T) {
	t.Parallel()
	h, err := harness.Lookup(harness.Claude)
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range []Meta{
		{Harness: "claude"},
		{Harness: "claude", Flow: tickets.FlowDesign},
		{Harness: "claude", Flow: tickets.FlowTriage, FlowKey: "proj"},
		{Harness: "claude", Ticket: "gx:e/01"},
		{Harness: "claude", FixOf: "gx:e/01"},
	} {
		opts := harness.LaunchOptions{SessionID: "s", Args: launchArgs(&meta, nil, nil, testFilesPrompt)}
		resume, _ := h.Resume("s", opts)
		for name, cmd := range map[string]harness.Command{"launch": h.Launch(opts), "resume": resume} {
			i := slices.Index(cmd.Args, "--append-system-prompt-file")
			if i < 0 || cmd.Args[i+1] != testFilesPrompt || slices.Contains(cmd.Args[i+1:], "--append-system-prompt-file") {
				t.Errorf("%+v %s args %q, want the files flag once", meta, name, cmd.Args)
			}
			if strings.Contains(strings.Join(cmd.Args, " "), "gx tickets ask") {
				t.Errorf("%+v %s args carry the ask text", meta, name)
			}
		}
	}
	args := launchArgs(&Meta{Harness: "codex"}, nil, nil, testFilesPrompt)
	if slices.Contains(args, "--append-system-prompt-file") {
		t.Errorf("codex args %q carry a Claude prompt flag", args)
	}
}

// A switch launches on a copy that took the switch, so the saved record keeps
// the old flags and the copy's args hold the new model and effort once, even
// over a configured --model.
func TestLaunchArgs_SwitchedCopyLeavesTheRecordAlone(t *testing.T) {
	t.Parallel()
	meta := Meta{Harness: "claude", Tier: "worker", Model: "sonnet", Effort: "medium"}
	launch := meta
	RecordSwitch(&launch, "opus", "high", "sonnet", nil, time.Now())
	got := launchArgs(&launch, []string{"--verbose", "--model", "x"}, nil, "")
	if want := []string{"--verbose", "--model", "opus", "--effort", "high"}; !slices.Equal(got[:len(want)], want) {
		t.Errorf("switched args = %q, want them to start with %q", got, want)
	}
	for _, flag := range []string{"--model", "--effort"} {
		if n := strings.Count(strings.Join(got, " "), flag+" "); n != 1 {
			t.Errorf("switched args = %q, want %s once, got %d", got, flag, n)
		}
	}
	if meta.Model != "sonnet" || meta.Effort != "medium" || meta.SwitchedByHand || meta.SwitchPrev != nil {
		t.Errorf("record = %+v, want it untouched by the copy's switch", meta)
	}
}

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
		{"claude-args hold a system prompt", Meta{Harness: "claude"}, []string{"--append-system-prompt", "mine"}, []string{"--append-system-prompt", "mine"}},
		{"claude model and effort", Meta{Harness: "claude", Model: "sonnet", Effort: "medium"}, claudeArgs, []string{"--verbose", "--model", "sonnet", "--effort", "medium"}},
		{"claude empty model", Meta{Harness: "claude", Tier: "worker", Effort: "high"}, nil, []string{"--effort", "high"}},
		{"claude empty effort", Meta{Harness: "claude", Tier: "worker", Model: "opus"}, nil, []string{"--model", "opus"}},
		{"claude-args hold --model", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x"}, []string{"--model", "x", "--effort", "high"}},
		{"claude-args hold --model=", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model=x"}, []string{"--model=x", "--effort", "high"}},
		{"claude-args hold --effort", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--effort", "low"}, []string{"--effort", "low", "--model", "opus"}},
		{"claude-args hold both", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x", "--effort", "low"}, []string{"--model", "x", "--effort", "low"}},
		{"claude tier with remote control and design", Meta{Harness: "claude", Flow: tickets.FlowDesign, Model: "opus", RemoteControl: "p:l/1"}, nil,
			[]string{"--disallowedTools", "AskUserQuestion", "--model", "opus", "--remote-control", "p:l/1"}},
		{"claude fix gets the ask hook", Meta{Harness: "claude", FixOf: "gx:e/01"}, nil, nil},
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
		if tc.meta.Harness == "claude" && !slices.Contains(tc.claudeArgs, "--append-system-prompt") {
			want = append(slices.Clone(want), "--append-system-prompt-file", testFilesPrompt)
		}
		if got := launchArgs(&tc.meta, tc.claudeArgs, roots, testFilesPrompt); !slices.Equal(got, want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, want)
		}
	}
	if !slices.Equal(claudeArgs, []string{"--verbose"}) {
		t.Errorf("configured claude args mutated: %v", claudeArgs)
	}
}

func TestLaunchArgs_HandSwitchWinsOverConfiguredFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		meta       Meta
		claudeArgs []string
		want       []string
	}{
		{"two-word flags", Meta{Harness: "claude", SwitchedByHand: true, Model: "opus", Effort: "high"}, []string{"--model", "x", "--verbose", "--effort", "low"}, []string{"--verbose", "--model", "opus", "--effort", "high"}},
		{"equals flags", Meta{Harness: "claude", SwitchedByHand: true, Model: "opus", Effort: "high"}, []string{"--model=x", "--effort=low"}, []string{"--model", "opus", "--effort", "high"}},
		{"empty effort keeps the configured flag", Meta{Harness: "claude", SwitchedByHand: true, Model: "opus"}, []string{"--model", "x", "--effort", "low"}, []string{"--effort", "low", "--model", "opus"}},
		{"without the flag a configured flag wins", Meta{Harness: "claude", Model: "opus", Effort: "high"}, []string{"--model", "x"}, []string{"--model", "x", "--effort", "high"}},
	} {
		want := append(slices.Clone(tc.want), harness.ClaudeAskHookArgs()...)
		want = append(want, "--append-system-prompt-file", testFilesPrompt)
		if got := launchArgs(&tc.meta, tc.claudeArgs, nil, testFilesPrompt); !slices.Equal(got, want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, want)
		}
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
		args := launchArgs(&tc.meta, nil, []string{"/repo/.git"}, testFilesPrompt)
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

func TestLaunchArgs_ClaudeAskHookOnPlainAndFixLaunchOnly(t *testing.T) {
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
		{"fix", Meta{Harness: "claude", FixOf: "gx:e/01"}, true},
	} {
		if got := askHookInSettings(t, launchArgs(&tc.meta, []string{"--verbose"}, nil, testFilesPrompt)); got != tc.want {
			t.Errorf("%s: ask hook installed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
