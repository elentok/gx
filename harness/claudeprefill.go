package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PrefillMaxBytes caps a prompt sent as --prefill, so a launch never nears the
// argv limit; a longer one is typed.
const PrefillMaxBytes = 32 << 10

// prefillProbeTimeout bounds the probe's run of claude; a hang reads as no
// support.
const prefillProbeTimeout = 5 * time.Second

// prefillProbe is the cache of ClaudeSupportsPrefill, keyed by the binary's
// path, size and mtime so an upgrade is probed again.
var prefillProbe sync.Map

// ClaudeSupportsPrefill reports whether the claude on PATH takes the hidden
// --prefill flag, which fills the input box at launch for the person (or gx)
// to submit. It is not in --help, so it runs `claude --prefill` with no value:
// a build that knows the flag complains the value is missing, one that does not
// complains the option is unknown. Any doubt reads as no support, so the caller
// types the prompt as before.
func ClaudeSupportsPrefill(env []string) bool {
	launchEnv := LaunchEnv(os.Environ(), env)
	path := lookPathIn("claude", LookupEnv(launchEnv, "PATH"))
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	key := path + "|" + info.ModTime().String() + "|" + strconv.FormatInt(info.Size(), 10)
	if v, ok := prefillProbe.Load(key); ok {
		return v.(bool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), prefillProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--prefill")
	cmd.Env = launchEnv
	out, _ := cmd.CombinedOutput()
	ok := ctx.Err() == nil && strings.Contains(string(out), "--prefill") && strings.Contains(string(out), "argument missing")
	prefillProbe.Store(key, ok)
	return ok
}

// lookPathIn finds the executable name in the PATH list pathEnv, the way the
// shell the launch runs in would; "" when there is none.
func lookPathIn(name, pathEnv string) string {
	for _, dir := range filepath.SplitList(pathEnv) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return p
		}
	}
	return ""
}
