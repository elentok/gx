// Package claudeenv keeps a developer's real Claude config out of test runs.
package claudeenv

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// Read the real caches before HOME changes: go derives its defaults from
	// HOME, and a temp HOME would make every run re-download the modules.
	if gocache, gomodcache, ok := goCaches(); ok {
		os.Setenv("GOCACHE", gocache)
		os.Setenv("GOMODCACHE", gomodcache)
	}
	os.Setenv("HOME", home)
	os.Setenv("CLAUDE_CONFIG_DIR", cfg)
	// Unset rather than redirect: tests that give themselves a HOME of their
	// own must still see it win over a package-wide XDG dir.
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		os.Unsetenv(k)
	}
	return func() {
		if err := removeTree(home); err != nil {
			fmt.Fprintf(os.Stderr, "claudeenv: removing %s: %v\n", home, err)
		}
	}
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

// removeTree deletes dir. Go leaves its module cache read-only, which makes
// os.RemoveAll fail, so each entry becomes owner-writable first. A dir is
// chmodded before WalkDir reads it.
func removeTree(dir string) error {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if d == nil {
			return nil
		}
		mode := os.FileMode(0o600)
		if d.IsDir() {
			mode = 0o700
		} else if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			mode |= info.Mode().Perm()
		}
		_ = os.Chmod(path, mode)
		return nil
	})
	return os.RemoveAll(dir)
}
