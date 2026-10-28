package brief

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// envOrEmpty returns env as a non-nil slice: a nil Cmd.Env would inherit the
// process's environment, and brief's children get only what the caller gave.
func envOrEmpty(env []string) []string {
	return append([]string{}, env...)
}

// lookPath resolves file against the PATH entry of env, as exec.LookPath does
// against the process's PATH.
func lookPath(env []string, file string) (string, error) {
	pathEnv := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			pathEnv = v
		}
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, file)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}
