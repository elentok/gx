package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// tempEnv returns an env (KEY=VALUE entries) rooted in a fresh temp HOME, with
// every XDG base dir under it. Pass it as the env field of deps, or to
// config.DirsFrom, so a test never reads or writes the real config, data or
// state dirs.
func tempEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
}

// testEnvs remembers the env made by envFor for each top-level test.
var testEnvs sync.Map // top-level test name -> []string

// envFor returns one tempEnv per top-level test: the first call makes it, later
// calls (including helpers like runIn and isolateTicketStore, and subtests of
// the same test) share it.
func envFor(t *testing.T) []string {
	t.Helper()
	key, _, _ := strings.Cut(t.Name(), "/")
	if env, ok := testEnvs.Load(key); ok {
		return env.([]string)
	}
	env := tempEnv(t)
	testEnvs.Store(key, env)
	t.Cleanup(func() { testEnvs.Delete(key) })
	return env
}

// setEnvFor sets key in the env envFor(t) returns from now on. It replaces
// t.Setenv for anything that runs through the deps env.
func setEnvFor(t *testing.T, key, value string) {
	t.Helper()
	top, _, _ := strings.Cut(t.Name(), "/")
	old := envFor(t)
	env := make([]string, 0, len(old)+1)
	for _, kv := range old {
		if !strings.HasPrefix(kv, key+"=") {
			env = append(env, kv)
		}
	}
	testEnvs.Store(top, append(env, key+"="+value))
}

// prependPath puts dir first on the PATH of envFor(t).
func prependPath(t *testing.T, dir string) {
	t.Helper()
	setEnvFor(t, "PATH", dir+string(os.PathListSeparator)+envValue(envFor(t), "PATH"))
}

// envValue returns the value of key in env, or "".
func envValue(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if rest, ok := strings.CutPrefix(kv, key+"="); ok {
			v = rest
		}
	}
	return v
}
