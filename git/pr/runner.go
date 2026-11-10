package pr

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// Runner runs gh under a caller-supplied environment. The server passes its
// own Config.Env so a test can point PATH at a fake gh; the package-level
// functions use a Runner over the process environment.
type Runner struct {
	Env []string
	Dir string
}

// Run executes gh and returns stdout without trailing newlines. A missing gh
// is reported as exec.ErrNotFound; a non-zero exit as *run.RunError.
func (r Runner) Run(args ...string) (string, error) {
	path, err := lookPathIn(r.Env, "gh")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = r.Env
	cmd.Dir = r.Dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &run.RunError{
				Args:   args,
				Dir:    r.Dir,
				Stdout: strings.TrimSpace(outBuf.String()),
				Stderr: strings.TrimSpace(errBuf.String()),
				Code:   exitErr.ExitCode(),
			}
		}
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimRight(outBuf.String(), "\r\n"), nil
}

// lookPathIn finds name on the PATH carried by env, not the process PATH
// (exec.Command resolves names against the latter).
func lookPathIn(env []string, name string) (string, error) {
	var pathVar string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			pathVar = v
		}
	}
	for _, dir := range filepath.SplitList(pathVar) {
		if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return p, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}
