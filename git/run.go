package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunError is returned when a git command exits with a non-zero status.
type RunError struct {
	Args   []string
	Dir    string
	Stdout string
	Stderr string
	Code   int
}

func (e *RunError) Error() string {
	return fmt.Sprintf("git %s failed (exit %d):\n%s\n%s",
		strings.Join(e.Args, " "), e.Code, e.Stdout, e.Stderr)
}

// run executes a git command in the given directory and returns trimmed stdout
// and stderr. Returns a *RunError if the command exits non-zero.
func run(dir string, args []string) (stdout, stderr string, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = strings.TrimRight(outBuf.String(), "\r\n")
	stderr = strings.TrimRight(errBuf.String(), "\r\n")
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return stdout, stderr, &RunError{
				Args:   args,
				Dir:    dir,
				Stdout: strings.TrimSpace(stdout),
				Stderr: strings.TrimSpace(stderr),
				Code:   exitErr.ExitCode(),
			}
		}
		return stdout, stderr, fmt.Errorf("git %s: %w", strings.Join(args, " "), runErr)
	}
	return stdout, stderr, nil
}

// runNoOptionalLocks executes a git command with optional index-refresh locks disabled.
// This is useful for read-only UI probes like `git status`, which can otherwise race with
// write operations such as stash/reset/rebase on macOS.
func runNoOptionalLocks(dir string, args []string) (stdout, stderr string, err error) {
	return run(dir, append([]string{"--no-optional-locks"}, args...))
}

// runAllowFail runs a git command and returns stdout, or "" if it fails.
func runAllowFail(dir string, args []string) string {
	out, _, _ := run(dir, args)
	return out
}

// joinOutput combines stdout and stderr into a single string, omitting empty halves.
func joinOutput(stdout, stderr string) string {
	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)
	if stdout == "" {
		return stderr
	}
	if stderr == "" {
		return stdout
	}
	return stdout + "\n" + stderr
}
