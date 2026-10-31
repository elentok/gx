package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/agentrunner/screen"
)

// codexHarness is the codex executable's side of a session. Codex has no
// session-id flag, so Launch passes none.
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

// Codex has no session-id flag, so a SessionStart hook hands gx the id: it
// copies the hook payload (JSON with session_id) to the file CodexSessionFileEnv
// names. The command is a constant, like the guards, and the path travels in
// the env.
const (
	CodexSessionFileEnv = "GX_CODEX_SESSION_FILE"
	// CodexSessionFile is the file's name in the agent dir.
	CodexSessionFile = "codex-session.json"
	// The temp file and rename keep the watcher from reading half a payload.
	CodexSessionStartCommand = `[ -n "$GX_CODEX_SESSION_FILE" ] && cat > "$GX_CODEX_SESSION_FILE.tmp" && mv "$GX_CODEX_SESSION_FILE.tmp" "$GX_CODEX_SESSION_FILE"`
)

func codexHookGroup(matcher, command string) string {
	m := ""
	if matcher != "" {
		m = "matcher=" + tomlString(matcher) + ","
	}
	return "{" + m + "hooks=[{type=\"command\",command=" + tomlString(command) + "}]}"
}

// codexHooks is the -c value holding gx's PreToolUse hooks.
func codexHooks() string {
	return "hooks.PreToolUse=[" + codexHookGroup("Bash", CodexGuardBashCommand) + "," + codexHookGroup(CodexQuestionTool, CodexGuardQuestionCommand) + "]"
}

// codexSessionHook is the -c value holding the SessionStart hook.
func codexSessionHook() string {
	return "hooks.SessionStart=[" + codexHookGroup("", CodexSessionStartCommand) + "]"
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
	// A server started inside a Claude shell carries a stale Claude session id,
	// which would make `gx agent context-window` see two harnesses.
	launch := slices.DeleteFunc(LaunchEnv(os.Environ(), nil), func(kv string) bool {
		return strings.HasPrefix(kv, "CLAUDE_CODE_SESSION_ID=")
	})
	return Command{Name: "codex", Args: slices.Clone(args), Env: append(launch, env...)}
}

// Watch reads the session file from the env of this process, which is the
// child's env: the PTY runner sets it on the holder.
func (h codexHarness) Watch(_ int, _ time.Time, scr Screen, _ <-chan struct{}) (Watcher, error) {
	w := newCodexWatcher(os.Getenv(CodexSessionFileEnv), func() bool {
		return scr != nil && h.StartupScreen(scr.ScrollbackTail(startupScrollback), scr.Rows()).Dialog != NoDialog
	})
	w.home = codexHomeFrom(os.Environ())
	return w, nil
}

// codexHomeFrom is Codex's home as the child reads it: CODEX_HOME, else
// ~/.codex; "" with neither set.
func codexHomeFrom(env []string) string {
	if dir := LookupEnv(env, "CODEX_HOME"); dir != "" {
		return dir
	}
	if home := LookupEnv(env, "HOME"); home != "" {
		return filepath.Join(home, ".codex")
	}
	return ""
}

// codexDialogs holds the text that names each startup dialog, lowercase. Only
// the trust wording is from real Codex; the rest is what the fake shows, since
// no recorded stream has a dialog. Update and model nudge are also switched
// off by flag. Every dialog is answered with Enter.
var codexDialogs = []struct {
	dialog Dialog
	text   []string
}{
	{TrustDialog, []string{"do you trust the contents", "trust this directory"}},
	{HooksReviewDialog, []string{"hooks review"}},
	{UpdateDialog, []string{"update available"}},
	{ModelNudgeDialog, []string{"model nudge"}},
}

func (codexHarness) StartupScreen(scrolled, rows []string) Startup {
	screenText := strings.ToLower(strings.Join(slices.Concat(scrolled, rows), "\n"))
	for _, d := range codexDialogs {
		if slices.ContainsFunc(d.text, func(t string) bool { return strings.Contains(screenText, t) }) {
			return Startup{Dialog: d.dialog, Key: "\r"}
		}
	}
	return Startup{}
}

// DisplayName drops the braille spinner Codex prefixes its title with while
// working.
func (codexHarness) DisplayName(raw []byte) (string, bool) {
	title, ok := screen.Title(raw)
	if !ok {
		return "", false
	}
	name := strings.TrimLeftFunc(title, func(r rune) bool { return isSpinner(r) || unicode.IsSpace(r) })
	return name, name != ""
}

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
		"-c", codexSessionHook(),
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
