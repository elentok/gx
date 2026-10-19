// Package claudeenv keeps a developer's real Claude config out of test runs.
package claudeenv

import (
	"os"
	"path/filepath"
)

// poisoned puts claude into vim mode, which changes how the ready-to-type step
// behaves. Anything that reads it fails loudly instead of passing on a host
// without vim mode.
const poisoned = `{"editorMode":"vim"}`

// Isolate points HOME at a scratch directory, unsets the XDG dirs and sets
// CLAUDE_CONFIG_DIR at a clean one inside it. The scratch HOME holds a
// poisoned ~/.claude.json and ~/.claude/settings.json, so code that resolves
// the config from HOME instead of CLAUDE_CONFIG_DIR trips the canary. Call it
// from TestMain and run the returned cleanup before os.Exit.
func Isolate() (cleanup func()) {
	home, err := os.MkdirTemp("", "gx-claudeenv")
	if err != nil {
		panic(err)
	}
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			panic(err)
		}
	}
	write(filepath.Join(home, ".claude.json"), poisoned)
	write(filepath.Join(home, ".claude", "settings.json"), poisoned)

	cfg := filepath.Join(home, "clean-claude-config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("CLAUDE_CONFIG_DIR", cfg)
	// Unset rather than redirect: tests that give themselves a HOME of their
	// own must still see it win over a package-wide XDG dir.
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		os.Unsetenv(k)
	}
	return func() { os.RemoveAll(home) }
}
