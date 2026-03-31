package git

import (
	"strings"
	"time"
)

// Stash saves the dirty state of the working directory onto the stash stack.
func Stash(dir string) (string, error) {
	var lastOut string
	for attempt := 0; attempt < 5; attempt++ {
		stdout, stderr, err := run(dir, []string{"stash"})
		lastOut = joinOutput(stdout, stderr)
		if err == nil {
			return lastOut, nil
		}
		if !isIndexLockBusyErr(err, lastOut) {
			return lastOut, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Final attempt result (lastOut) already captured.
	stdout, stderr, err := run(dir, []string{"stash"})
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
