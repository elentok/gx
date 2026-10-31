package harness_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
)

func TestCodexArgs_Shape(t *testing.T) {
	t.Parallel()
	roots := []string{"/repo/.git/worktrees/wt", "/repo/.git", "/store/proj", "/store/proj/epic"}
	evolve := append(slices.Clone(roots), "/store/gx/follow-ups", "/store/gx")
	for _, tc := range []struct {
		name string
		in   harness.CodexArgsInput
	}{
		{"queue implement", harness.CodexArgsInput{Model: "gpt-6.1-sol", Effort: "medium", WritableRoots: roots}},
		{"queue evolve", harness.CodexArgsInput{Model: "gpt-6.1-sol", Effort: "medium", WritableRoots: evolve}},
		{"agents tab", harness.CodexArgsInput{WritableRoots: roots[:3]}},
		{"design with model only", harness.CodexArgsInput{Model: "gpt-6-astra", WritableRoots: roots[:3]}},
		{"fix agent with effort only", harness.CodexArgsInput{Effort: "high", WritableRoots: roots[:3]}},
		{"no roots", harness.CodexArgsInput{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := harness.CodexArgs(tc.in)
			joined := strings.Join(args, " ")
			for _, want := range []string{
				"-c check_for_update_on_startup=false",
				"--sandbox workspace-write",
				"--ask-for-approval on-request",
				"--approve-for-me",
				"-c sandbox_workspace_write.network_access=true",
				"-c disable_paste_burst=true",
				"--dangerously-bypass-hook-trust",
				`-c hooks.PermissionRequest=[{hooks=[{type="command",command="gx agent permission-request"}]}]`,
			} {
				if strings.Count(joined, want) != 1 {
					t.Errorf("%q appears %d times in %q, want once", want, strings.Count(joined, want), joined)
				}
			}
			for _, never := range []string{"--add-dir", "danger-full-access", "untrusted"} {
				if strings.Contains(joined, never) {
					t.Errorf("%q in %q", never, joined)
				}
			}
			if len(tc.in.WritableRoots) == 0 {
				if strings.Contains(joined, "writable_roots") {
					t.Errorf("writable_roots in %q with no roots", joined)
				}
			} else {
				i := slices.IndexFunc(args, func(a string) bool { return strings.HasPrefix(a, "sandbox_workspace_write.writable_roots=") })
				if i < 1 || args[i-1] != "-c" {
					t.Fatalf("no -c writable_roots in %q", joined)
				}
				for _, r := range tc.in.WritableRoots {
					if !strings.Contains(args[i], `"`+r+`"`) {
						t.Errorf("writable_roots %s lacks %s", args[i], r)
					}
				}
			}
			if got := slices.Contains(args, "--model"); got != (tc.in.Model != "") {
				t.Errorf("--model present = %v, model = %q", got, tc.in.Model)
			}
			if got := strings.Contains(joined, "model_reasoning_effort="); got != (tc.in.Effort != "") {
				t.Errorf("effort flag present = %v, effort = %q", got, tc.in.Effort)
			}
		})
	}
}

func TestCodexArgs_HookSet(t *testing.T) {
	t.Parallel()
	args := harness.CodexArgs(harness.CodexArgsInput{})
	want := `hooks.PreToolUse=[` +
		`{matcher="Bash",hooks=[{type="command",command="gx agent guard-bash --harness codex"}]},` +
		`{matcher="request_user_input",hooks=[{type="command",command="gx agent guard-question"}]}]`
	i := slices.Index(args, want)
	if i < 1 || args[i-1] != "-c" {
		t.Errorf("args %q lack -c %s", args, want)
	}
	// The prompt belongs to a fresh Launch, never the shared args Resume reuses.
	if strings.Contains(strings.Join(args, " "), "developer_instructions") {
		t.Errorf("CodexArgs carries developer_instructions: %q", args)
	}
}

func TestCodexArgs_RootsAreDeduped(t *testing.T) {
	t.Parallel()
	args := strings.Join(harness.CodexArgs(harness.CodexArgsInput{WritableRoots: []string{"/a/.git", "/a/.git", "/b"}}), " ")
	if want := `writable_roots=["/a/.git","/b"]`; !strings.Contains(args, want) {
		t.Errorf("args = %q, want %q", args, want)
	}
}

func TestCodexHarness_LaunchDropsTheStaleClaudeSessionID(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "stale")
	h, err := harness.Lookup(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range h.Launch(harness.LaunchOptions{}).Env {
		if strings.HasPrefix(kv, "CLAUDE_CODE_SESSION_ID=") {
			t.Errorf("Codex launch env carries %q", kv)
		}
	}
}

func TestCodexHarness_LaunchAndResume(t *testing.T) {
	t.Parallel()
	h, err := harness.Lookup(harness.Codex)
	if err != nil {
		t.Fatal(err)
	}
	opts := harness.LaunchOptions{SessionID: "ignored", Args: []string{"--model", "m"}}
	launch := h.Launch(opts)
	wantPrompt := "developer_instructions=" + harness.TOMLString(harness.GXSystemPromptFor(harness.Codex))
	if launch.Name != "codex" || !slices.Equal(launch.Args, []string{"--model", "m", "-c", wantPrompt}) {
		t.Errorf("Launch = %+v, want codex --model m plus the developer_instructions flag, no session-id flag", launch)
	}
	if strings.Contains(strings.Join(launch.Args, " "), "ignored") {
		t.Errorf("Launch passed the session id: %v", launch.Args)
	}
	resume, ok := h.Resume("abc", opts)
	if !ok || resume.Name != "codex" || !slices.Equal(resume.Args, []string{"resume", "abc", "--model", "m"}) {
		t.Errorf("Resume = %+v, %v, want codex resume abc --model m", resume, ok)
	}
	opts.Cwd = "/work/dir"
	resume, _ = h.Resume("abc", opts)
	if !slices.Equal(resume.Args, []string{"resume", "abc", "--cd", "/work/dir", "--model", "m"}) {
		t.Errorf("Resume with a cwd = %q, want resume abc --cd /work/dir --model m", resume.Args)
	}
	if _, err := h.Watch(0, time.Time{}, nil, nil); err != nil {
		t.Errorf("Watch error = %v, want a watcher", err)
	}
	if name, ok := h.DisplayName([]byte("x")); ok || name != "" {
		t.Errorf("DisplayName without a title = %q, %v, want none", name, ok)
	}
	if name, ok := h.DisplayName([]byte("\x1b]0;⠋ Run tests | gx\a")); !ok || name != "Run tests | gx" {
		t.Errorf("DisplayName = %q, %v, want the title without its spinner", name, ok)
	}
	if got := h.StartupScreen(nil, nil); got != (harness.Startup{}) {
		t.Errorf("StartupScreen = %+v, want zero", got)
	}
}
