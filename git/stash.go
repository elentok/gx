package git

import (
	"strconv"
	"strings"
	"time"
)

// StashEntry represents one entry from `git stash list`.
type StashEntry struct {
	Index     int
	Ref       string // e.g. "stash@{0}"
	Message   string // e.g. "On main: my stash"
	Timestamp time.Time
}

// StashList returns all stash entries for the repo at dir, newest first.
func StashList(dir string) ([]StashEntry, error) {
	out, _, err := run(dir, []string{"stash", "list", "--format=format:%gd%x09%at%x09%gs"})
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var entries []StashEntry
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		ref := parts[0]
		index := parseStashIndex(ref)
		ts, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		entries = append(entries, StashEntry{
			Index:     index,
			Ref:       ref,
			Message:   parts[2],
			Timestamp: time.Unix(ts, 0),
		})
	}
	return entries, nil
}

// parseStashIndex extracts N from "stash@{N}".
func parseStashIndex(ref string) int {
	open := strings.Index(ref, "{")
	close := strings.Index(ref, "}")
	if open < 0 || close <= open {
		return -1
	}
	n, err := strconv.Atoi(ref[open+1 : close])
	if err != nil {
		return -1
	}
	return n
}

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

// StashApply applies a specific stash entry without removing it from the stack.
func StashApply(dir, ref string) (string, error) {
	stdout, stderr, err := run(dir, []string{"stash", "apply", ref})
	return joinOutput(stdout, stderr), err
}

// StashPopRef applies and removes a specific stash entry from the stack.
func StashPopRef(dir, ref string) (string, error) {
	stdout, stderr, err := run(dir, []string{"stash", "pop", ref})
	return joinOutput(stdout, stderr), err
}

// StashDrop removes a specific stash entry from the stack without applying it.
func StashDrop(dir, ref string) (string, error) {
	stdout, stderr, err := run(dir, []string{"stash", "drop", ref})
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
