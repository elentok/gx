package git

import "github.com/elentok/gx/git/internal/run"

// The run mechanism lives in git/internal/run so the sibling packages
// (git/pr, git/stage, git/history) can share it. These names keep the public
// git API unchanged for callers outside git.

type (
	// RunError is returned when a git command exits with a non-zero status.
	RunError = run.RunError
	// RunOptions tunes RunWith.
	RunOptions = run.RunOptions
)

// Run executes git in dir with exactly env as its environment.
func Run(dir string, env []string, args ...string) (stdout, stderr string, err error) {
	return run.Run(dir, env, args...)
}

// RunWith is Run with stdin, output writers and allowed exit codes.
func RunWith(dir string, env []string, o RunOptions, args ...string) (stdout, stderr string, err error) {
	return run.RunWith(dir, env, o, args...)
}

// RunRaw is Run for output that must stay byte-exact.
func RunRaw(dir string, env []string, args ...string) (stdout []byte, stderr string, err error) {
	return run.RunRaw(dir, env, args...)
}

// NonInteractiveEnv is the process environment with git prompts disabled.
func NonInteractiveEnv() []string { return run.NonInteractiveEnv() }
