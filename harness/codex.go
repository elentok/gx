package harness

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// codexHarness is the codex executable's side of a session. Codex has no
// session-id flag, so Launch passes none; the PTY runner starts only Claude
// until the Codex watch lands.
type codexHarness struct{}

// CodexQuestionTool is Codex's tool for asking a person, which gx blocks in
// favour of `gx tickets ask`.
const CodexQuestionTool = "request_user_input"

// The hook commands are constants: Codex trusts a hook by its hash, so a
// command that varies per launch would need a fresh review each time.
const (
	CodexGuardBashCommand     = "gx agent guard-bash --harness codex"
	CodexGuardQuestionCommand = "gx agent guard-question"
)

// codexHooks is the -c value holding gx's PreToolUse hooks.
func codexHooks() string {
	group := func(matcher, command string) string {
		return "{matcher=" + tomlString(matcher) + ",hooks=[{type=\"command\",command=" + tomlString(command) + "}]}"
	}
	return "hooks.PreToolUse=[" + group("Bash", CodexGuardBashCommand) + "," + group(CodexQuestionTool, CodexGuardQuestionCommand) + "]"
}

// Launch adds the GX system prompt, on a fresh session only: Resume must not
// carry it again (an instruction survives a resume).
func (codexHarness) Launch(opts LaunchOptions) Command {
	args := append(slices.Clone(opts.Args), "-c", "developer_instructions="+tomlString(GXSystemPromptFor(Codex)))
	return codexCommand(args, opts.Env)
}

// Resume takes no rollout check or --cd yet.
func (codexHarness) Resume(conversationID string, opts LaunchOptions) (Command, bool) {
	return codexCommand(append([]string{"resume", conversationID}, opts.Args...), opts.Env), true
}

func codexCommand(args, env []string) Command {
	return Command{Name: "codex", Args: slices.Clone(args), Env: LaunchEnv(os.Environ(), env)}
}

func (codexHarness) Watch(int, time.Time, Screen, <-chan struct{}) (Watcher, error) {
	return nil, errors.New("codex session watching is not built yet")
}

func (codexHarness) StartupScreen(scrolled, rows []string) Startup { return Startup{} }

func (codexHarness) DisplayName([]byte) (string, bool) { return "", false }

// CodexArgsInput is what varies between Codex launches.
type CodexArgsInput struct {
	// Model and Effort are passed only when set.
	Model, Effort string
	// WritableRoots are absolute dirs the sandbox may write besides the cwd:
	// the worktree's git dir, the common git dir, the ticket store and the
	// epic dir, plus an evolve ticket's extra dirs.
	WritableRoots []string
}

// CodexArgs is the one launch shape every Codex launch uses (queue, Agents
// tab, design, Fix agent, Investigate, code-review, verify, evolve). It never
// uses --add-dir (not accepted by `codex resume`) or danger-full-access.
//
// --dangerously-bypass-hook-trust is what lets the inline hooks run
// unreviewed. It also runs any hooks.json a repo holds without review, a
// known risk the person accepts by launching Codex in that repo.
func CodexArgs(in CodexArgsInput) []string {
	args := []string{
		"-c", "check_for_update_on_startup=false",
		"--sandbox", "workspace-write",
		"--ask-for-approval", "on-request",
		"--approve-for-me",
		"-c", "sandbox_workspace_write.network_access=true",
		"-c", "disable_paste_burst=true",
		"--dangerously-bypass-hook-trust",
		"-c", codexHooks(),
	}
	if roots := dedupe(in.WritableRoots); len(roots) > 0 {
		args = append(args, "-c", "sandbox_workspace_write.writable_roots="+tomlStringArray(roots))
	}
	if in.Model != "" {
		args = append(args, "--model", in.Model)
	}
	if in.Effort != "" {
		// Codex has no effort flag; -c is the only route, and gx does not
		// manage a codex profile file to get one.
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", in.Effort))
	}
	return args
}

func dedupe(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// tomlStringArray renders paths as a TOML array of basic strings.
func tomlStringArray(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
