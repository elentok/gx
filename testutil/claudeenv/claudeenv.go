// Package claudeenv keeps a developer's real Claude config out of test runs.
package claudeenv

import (
	"os"
	"os/exec"
	"path/filepath"
)

// poisoned puts claude into vim mode, which changes how the ready-to-type step
// behaves. Anything that reads it fails loudly instead of passing on a host
// without vim mode.
const poisoned = `{"editorMode":"vim"}`

// In fills the existing dir home with a poisoned ~/.claude.json and
// ~/.claude/settings.json and a clean config dir, and returns the env entries
// (KEY=VALUE, for server.Config.Env or exec.Cmd.Env) that point at them. Code
// that resolves the config from HOME instead of CLAUDE_CONFIG_DIR trips the
// canary. The XDG dirs come back empty, which config's xdgBase ignores.
func In(home string) []string {
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
	env := []string{
		"HOME=" + home,
		"CLAUDE_CONFIG_DIR=" + cfg,
		"XDG_CONFIG_HOME=", "XDG_STATE_HOME=", "XDG_DATA_HOME=", "XDG_CACHE_HOME=",
	}
	// go derives its cache defaults from HOME, and a temp HOME would make every
	// child go command re-download the modules into a read-only tree.
	if gocache, gomodcache, ok := goCaches(); ok {
		env = append(env, "GOCACHE="+gocache, "GOMODCACHE="+gomodcache)
	}
	return env
}

// goCaches returns the GOCACHE and GOMODCACHE that go resolves right now.
func goCaches() (gocache, gomodcache string, ok bool) {
	out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", false
	}
	return lines[0], lines[1], true
}
