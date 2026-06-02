package git

import (
	"strings"
	"time"
)

// Stash saves the dirty state of the working directory onto the stash stack.
func Stash(dir string) (string, error) {
	return runGitWithIndexLockRetry(dir, []string{"stash"})
}

// StashPush saves changes onto the stash stack with an optional name. When
// stagedOnly is true only the staged changes are stashed (git >= 2.35); the
// unstaged working-tree changes are left in place. An empty name lets git
// generate its usual "WIP on <branch>" message.
func StashPush(dir string, name string, stagedOnly bool) (string, error) {
	args := []string{"stash", "push"}
	if stagedOnly {
		args = append(args, "--staged")
	}
	if name = strings.TrimSpace(name); name != "" {
		args = append(args, "-m", name)
	}
	return runGitWithIndexLockRetry(dir, args)
}

// runGitWithIndexLockRetry runs a git command, retrying a handful of times when
// the failure is a busy index.lock (another git process holding the lock).
func runGitWithIndexLockRetry(dir string, args []string) (string, error) {
	var lastOut string
	for range 5 {
		stdout, stderr, err := run(dir, args)
		lastOut = joinOutput(stdout, stderr)
		if err == nil {
			return lastOut, nil
		}
		if !isIndexLockBusyErr(err, lastOut) {
			return lastOut, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Final attempt after the retry budget is exhausted.
	stdout, stderr, err := run(dir, args)
	lastOut = joinOutput(stdout, stderr)
	return lastOut, err
}

// StashPop applies the most recent stash and removes it from the stash stack.
func StashPop(dir string) (string, error) {
	stdout, stderr, err := run(dir, []string{"stash", "pop"})
	return joinOutput(stdout, stderr), err
}

// Rebase rebases the current branch onto the given ref.
func Rebase(dir string, onto string) (string, error) {
	stdout, stderr, err := run(dir, []string{"rebase", onto})
	return joinOutput(stdout, stderr), err
}

func isIndexLockBusyErr(err error, output string) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(output + "\n" + err.Error())
	return strings.Contains(s, "index.lock") || strings.Contains(s, "another git process seems to be running")
}
