package harness

import (
	"os"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/screen"
)

type claudeHarness struct{}

func (claudeHarness) Launch(opts LaunchOptions) Command {
	return claudeCommand(append([]string{"--session-id", opts.SessionID}, opts.Args...), opts.Env)
}

func (claudeHarness) Resume(conversationID string, opts LaunchOptions) (Command, bool) {
	return claudeCommand(append([]string{"--resume", conversationID}, opts.Args...), opts.Env), true
}

func claudeCommand(args, env []string) Command {
	return Command{Name: "claude", Args: args, Env: LaunchEnv(os.Environ(), env)}
}

func (claudeHarness) Watch(pid int, spawned time.Time, stop <-chan struct{}) (Watcher, error) {
	dir, err := ClaudeSessionsDir()
	if err != nil {
		return nil, err
	}
	w := newPidWatcher(dir, pid, spawned)
	go w.watch(stop)
	return w, nil
}

func (claudeHarness) StartupScreen(scrolled, rows []string) Startup {
	if claudeui.ClassifyStartup(claudeui.ScreenText(scrolled, rows)) == claudeui.StartupTrust {
		return Startup{Dialog: TrustDialog, Key: claudeui.TrustKey(rows)}
	}
	return Startup{}
}

func (claudeHarness) DisplayName(raw []byte) (string, bool) {
	return screen.Title(raw)
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
