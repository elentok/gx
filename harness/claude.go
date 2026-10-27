package harness

import (
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/screen"
)

// claudeHarness reads claude's config (vim mode) from env; nil is the process
// env.
type claudeHarness struct{ env []string }

func (claudeHarness) Launch(opts LaunchOptions) Command {
	return claudeCommand(append([]string{"--session-id", opts.SessionID}, opts.Args...), opts.Env)
}

func (claudeHarness) Resume(conversationID string, opts LaunchOptions) (Command, bool) {
	return claudeCommand(append([]string{"--resume", conversationID}, opts.Args...), opts.Env), true
}

// claudeCommand runs every claude agent in auto permission mode, so a launch
// never stalls on a permission prompt. The flag goes right after the session
// flag pair unless the caller already passed one.
func claudeCommand(args, env []string) Command {
	if !slices.Contains(args, "--permission-mode") {
		args = slices.Insert(args, 2, "--permission-mode", "auto")
	}
	return Command{Name: "claude", Args: args, Env: LaunchEnv(os.Environ(), env)}
}

func (h claudeHarness) Watch(pid int, spawned time.Time, scr Screen, stop <-chan struct{}) (Watcher, error) {
	dir, err := ClaudeSessionsDir()
	if err != nil {
		return nil, err
	}
	w := newPidWatcher(dir, pid, spawned)
	w.startup = func() bool {
		return h.StartupScreen(scr.ScrollbackTail(startupScrollback), scr.Rows()).Dialog != NoDialog
	}
	w.inputBox = func() bool { return claudeui.InputBox(scr.Rows()) }
	go w.watch(stop)
	return w, nil
}

// startupScrollback is how much scrolled-off text a pre-session dialog can
// have: a narrow PTY wraps it past the visible rows.
const startupScrollback = 50

func (claudeHarness) StartupScreen(scrolled, rows []string) Startup {
	if claudeui.ClassifyStartup(claudeui.ScreenText(scrolled, rows)) == claudeui.StartupTrust {
		return Startup{Dialog: TrustDialog, Key: claudeui.TrustKey(rows)}
	}
	return Startup{}
}

// DisplayName drops the status glyph claude prefixes its title with (✳
// idle, a braille spinner while working): the Agents tab shows the state
// with its own icon.
func (claudeHarness) DisplayName(raw []byte) (string, bool) {
	title, ok := screen.Title(raw)
	if !ok {
		return "", false
	}
	name := strings.TrimLeftFunc(title, func(r rune) bool { return unicode.IsSymbol(r) || unicode.IsSpace(r) })
	return name, name != ""
}

// LaunchEnv drops only the vars that make claude think it is nested inside
// another claude session, which would stop it saving its transcript and its
// pid status file.
func LaunchEnv(base, extra []string) []string {
	env := slices.DeleteFunc(slices.Clone(base), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return name == "CLAUDECODE" || name == "CLAUDE_CODE_ENTRYPOINT" || name == "CLAUDE_CODE_CHILD_SESSION"
	})
	env = append(env, "DISABLE_AUTOUPDATER=1")
	return append(env, extra...)
}
