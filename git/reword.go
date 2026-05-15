package git

import (
	"fmt"
	"os"
)

// RewordHead amends the HEAD commit message only, leaving any staged content untouched.
func RewordHead(root, message string) (string, error) {
	tmpFile, err := writeTempRewordMessage(message)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile)
	stdout, stderr, err := run(root, []string{"commit", "--amend", "--only", "-F", tmpFile})
	return joinOutput(stdout, stderr), err
}

// RewordCommit rewrites the message of an arbitrary non-HEAD commit via interactive rebase.
// --autostash handles both staged and unstaged working-tree changes automatically.
func RewordCommit(root, hash, message string) (string, error) {
	msgFile, err := writeTempRewordMessage(message)
	if err != nil {
		return "", err
	}
	defer os.Remove(msgFile)

	// Line 1 of the rebase TODO is always the target commit when rebasing from hash^.
	seqEditor, err := writeTempRewordScript("#!/bin/sh\nsed -i.bak '1s/^pick /reword /' \"$1\"\nrm -f \"$1.bak\"\n")
	if err != nil {
		os.Remove(msgFile)
		return "", err
	}
	defer os.Remove(seqEditor)

	// Copy our pre-written message into git's temp commit-msg file.
	msgEditor, err := writeTempRewordScript("#!/bin/sh\ncp \"$GX_REWORD_MSG\" \"$1\"\n")
	if err != nil {
		return "", err
	}
	defer os.Remove(msgEditor)

	return runWithExtraEnv(root,
		[]string{"rebase", "-i", "--autostash", hash + "^"},
		[]string{
			"GIT_SEQUENCE_EDITOR=" + seqEditor,
			"GIT_EDITOR=" + msgEditor,
			"GX_REWORD_MSG=" + msgFile,
		},
	)
}

func writeTempRewordMessage(message string) (string, error) {
	f, err := os.CreateTemp("", "gx-reword-msg-*")
	if err != nil {
		return "", fmt.Errorf("create temp message file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(message); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write temp message file: %w", err)
	}
	return f.Name(), nil
}

func writeTempRewordScript(content string) (string, error) {
	f, err := os.CreateTemp("", "gx-reword-script-*")
	if err != nil {
		return "", fmt.Errorf("create temp script: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0700); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("chmod temp script: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write temp script: %w", err)
	}
	return f.Name(), nil
}
