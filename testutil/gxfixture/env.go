package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/claudeenv"
)

// hostPathWithoutCodex is the caller's PATH with a real codex hidden, so a fixture
// without --fakecodex looks like a host without Codex. A directory that holds
// codex is replaced by a directory of symlinks to everything else in it, which
// keeps git and the other tools beside it reachable.
func hostPathWithoutCodex(root string) string {
	var out []string
	for i, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Lstat(filepath.Join(dir, "codex")); err != nil {
			out = append(out, dir)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		shadow := filepath.Join(root, "hostbin", strconv.Itoa(i))
		if os.MkdirAll(shadow, 0o755) != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() != "codex" {
				_ = os.Symlink(filepath.Join(dir, e.Name()), filepath.Join(shadow, e.Name()))
			}
		}
		out = append(out, shadow)
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// isolatedEnv is the process environment with every variable that could reach
// the real HOME, config, state or Claude config replaced, and the ones that
// make a nested agent act differently dropped.
func isolatedEnv(root, binDir string) (env, cliEnv []string) {
	drop := map[string]bool{"HOME": true, "PATH": true, "GIT_CONFIG_GLOBAL": true, "AI_AGENT": true}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] && !strings.HasPrefix(k, "XDG_") && !strings.HasPrefix(k, "GX_") &&
			!strings.HasPrefix(k, "CLAUDE") && !strings.HasPrefix(k, "HERDR_") {
			env = append(env, kv)
		}
	}
	cliEnv = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
	}
	// HOME is replaced, so Playwright would look for its browsers in the empty
	// temp HOME: name the real cache.
	if p := playwrightBrowsersPath(); p != "" {
		cliEnv = append(cliEnv, "PLAYWRIGHT_BROWSERS_PATH="+p)
	}
	// HOME is replaced, so go would download the modules and toolchain again into
	// the temp HOME's read-only tree, 250 MB per fixture that a killed run leaves
	// behind. cliEnv carries these too: the CLI and fake agents run go as well.
	// -modcacherw keeps whatever does land there deletable.
	cliEnv = append(cliEnv, claudeenv.GoCacheEnv()...)
	cliEnv = append(cliEnv, "GOFLAGS=-modcacherw")
	env = append(env, cliEnv...)
	env = append(env,
		"PATH="+binDir+string(os.PathListSeparator)+hostPathWithoutCodex(root),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		osnotify.OffEnv+"=off",
		// The launch record a launch prompt waits for.
		agentfake.TUILaunchRecordEnv+"=1",
		"GIT_AUTHOR_NAME=gxfixture", "GIT_AUTHOR_EMAIL=gxfixture@example.com",
		"GIT_COMMITTER_NAME=gxfixture", "GIT_COMMITTER_EMAIL=gxfixture@example.com",
	)
	return git.WithMaintenanceOff(env), cliEnv
}

// playwrightBrowsersPath is the caller's PLAYWRIGHT_BROWSERS_PATH, else
// Playwright's default cache under the real home ("" when it cannot be told).
func playwrightBrowsersPath() string {
	if p := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "ms-playwright")
	}
	return filepath.Join(home, ".cache", "ms-playwright")
}
