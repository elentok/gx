package git

import (
	"bytes"
	"fmt"
	"io"
	"os"
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
	return runEnv(dir, NonInteractiveEnv(), args)
}

// RunOptions tunes RunWith.
type RunOptions struct {
	Stdin io.Reader
	// Stdout and Stderr, when set, receive the stream instead of the captured
	// copy RunWith returns.
	Stdout, Stderr io.Writer
	// AllowExit lists non-zero exit codes that count as success.
	AllowExit map[int]bool
}

// Run executes git in dir with exactly env as its environment and returns
// trimmed stdout and stderr. Returns a *RunError if the command exits non-zero.
func Run(dir string, env []string, args ...string) (stdout, stderr string, err error) {
	return RunWith(dir, env, RunOptions{}, args...)
}

// RunWith is Run with stdin, output writers and allowed exit codes.
func RunWith(dir string, env []string, o RunOptions, args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	runErr := runCore(dir, env, o, &outBuf, &errBuf, args)
	stdout = strings.TrimRight(outBuf.String(), "\r\n")
	stderr = strings.TrimRight(errBuf.String(), "\r\n")
	return stdout, stderr, wrapRunErr(runErr, dir, args, stdout, stderr, o.AllowExit)
}

// RunRaw is Run for output that must stay byte-exact (NUL-separated lists,
// blobs): stdout is not trimmed.
func RunRaw(dir string, env []string, args ...string) (stdout []byte, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	runErr := runCore(dir, env, RunOptions{}, &outBuf, &errBuf, args)
	stderr = strings.TrimRight(errBuf.String(), "\r\n")
	return outBuf.Bytes(), stderr, wrapRunErr(runErr, dir, args, outBuf.String(), stderr, nil)
}

// runCore is the one place gx starts a git child.
func runCore(dir string, env []string, o RunOptions, outBuf, errBuf *bytes.Buffer, args []string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if o.Stdin != nil {
		cmd.Stdin = o.Stdin
	}
	cmd.Stdout, cmd.Stderr = outBuf, errBuf
	if o.Stdout != nil {
		cmd.Stdout = o.Stdout
	}
	if o.Stderr != nil {
		cmd.Stderr = o.Stderr
	}
	return cmd.Run()
}

func wrapRunErr(runErr error, dir string, args []string, stdout, stderr string, allow map[int]bool) error {
	if runErr == nil {
		return nil
	}
	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), runErr)
	}
	if allow[exitErr.ExitCode()] {
		return nil
	}
	return &RunError{
		Args:   args,
		Dir:    dir,
		Stdout: strings.TrimSpace(stdout),
		Stderr: strings.TrimSpace(stderr),
		Code:   exitErr.ExitCode(),
	}
}

// runEnv is run with an explicit environment.
func runEnv(dir string, env, args []string) (stdout, stderr string, err error) {
	return Run(dir, env, args...)
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

// runRawAllowFail runs a git command and returns raw stdout bytes, or nil if it
// fails. Unlike run/runAllowFail, it does not trim trailing newlines, so it is
// safe for binary output such as `cat-file -p` on a blob.
func runRawAllowFail(dir string, args []string) []byte {
	out, _, err := RunRaw(dir, NonInteractiveEnv(), args...)
	if err != nil {
		return nil
	}
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

func NonInteractiveEnv() []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}
