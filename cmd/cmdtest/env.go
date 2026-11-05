// Package cmdtest is the shared test plumbing of the cmd packages: per-test
// temp envs, the TestMain hooks, a mini root command and the project-lock wait
// hook override. It imports testing, so only test files import it.
package cmdtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// GoBuildCache is the real Go build cache. A temp HOME would give each child
// `go test` a cold cache, so every one recompiles the standard library.
var GoBuildCache = sync.OnceValue(func() string {
	out, err := exec.Command("go", "env", "GOCACHE").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
})

// sharedGxDir holds the gx binary built once for the whole test run (MainWithGx
// removes it). Building it per test made the first test of a loaded run wait
// past the hang limit for a cold compile.
var sharedGxDir string

var buildSharedGx = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "gx-bin")
	if err != nil {
		return "", err
	}
	sharedGxDir = dir
	out, err := exec.Command("go", "build", "-o", filepath.Join(dir, "gx"), "github.com/elentok/gx").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build: %w\n%s", err, out)
	}
	return dir, nil
})

// SharedGx returns the dir holding the shared gx binary, for PrependPath.
func SharedGx(t *testing.T) string {
	t.Helper()
	dir, err := buildSharedGx()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TempEnv returns an env (KEY=VALUE entries) rooted in a fresh temp HOME, with
// every XDG base dir under it. Pass it as the Env field of cmdbase.Env, or to
// config.DirsFrom, so a test never reads or writes the real config, data or
// state dirs.
func TempEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return []string{
		"HOME=" + home,
		"GOCACHE=" + GoBuildCache(),
		"PATH=" + os.Getenv("PATH"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
}

// testEnvs remembers the env made by EnvFor for each top-level test.
var testEnvs sync.Map // top-level test name -> []string

// EnvFor returns one TempEnv per top-level test: the first call makes it, later
// calls (including helpers and subtests of the same test) share it.
func EnvFor(t *testing.T) []string {
	t.Helper()
	key, _, _ := strings.Cut(t.Name(), "/")
	if env, ok := testEnvs.Load(key); ok {
		return env.([]string)
	}
	env := TempEnv(t)
	testEnvs.Store(key, env)
	t.Cleanup(func() { testEnvs.Delete(key) })
	return env
}

// SetEnvFor sets key in the env EnvFor(t) returns from now on. It replaces
// t.Setenv for anything that runs through the Env.
func SetEnvFor(t *testing.T, key, value string) {
	t.Helper()
	top, _, _ := strings.Cut(t.Name(), "/")
	old := EnvFor(t)
	env := make([]string, 0, len(old)+1)
	for _, kv := range old {
		if !strings.HasPrefix(kv, key+"=") {
			env = append(env, kv)
		}
	}
	testEnvs.Store(top, append(env, key+"="+value))
}

// PrependPath puts dir first on the PATH of EnvFor(t).
func PrependPath(t *testing.T, dir string) {
	t.Helper()
	SetEnvFor(t, "PATH", dir+string(os.PathListSeparator)+EnvValue(EnvFor(t), "PATH"))
}

// EnvValue returns the value of key in env, or "".
func EnvValue(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if rest, ok := strings.CutPrefix(kv, key+"="); ok {
			v = rest
		}
	}
	return v
}
