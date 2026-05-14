package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// StagedFiles returns the paths of files with staged index changes.
func StagedFiles(root string) ([]string, error) {
	files, err := ListStageFiles(root)
	if err != nil {
		return nil, err
	}
	var staged []string
	for _, f := range files {
		if f.HasStagedChanges() {
			staged = append(staged, f.Path)
		}
	}
	return staged, nil
}

// IsCommitPushed reports whether hash appears in any remote tracking branch.
func IsCommitPushed(root, hash string) (bool, error) {
	out, _, err := run(root, []string{"branch", "-r", "--contains", hash})
	if err != nil {
		if _, ok := err.(*RunError); ok {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// IsHEAD reports whether hash resolves to the current HEAD commit.
func IsHEAD(root, hash string) (bool, error) {
	headHash, _, err := run(root, []string{"rev-parse", "HEAD"})
	if err != nil {
		return false, fmt.Errorf("resolve HEAD: %w", err)
	}
	return strings.TrimSpace(headHash) == strings.TrimSpace(hash), nil
}

// AmendHead amends the HEAD commit with the currently staged changes.
func AmendHead(root string) (string, error) {
	stdout, stderr, err := run(root, []string{"commit", "--amend", "--no-edit"})
	return joinOutput(stdout, stderr), err
}

// CommitFixup creates a fixup commit targeting the given hash.
func CommitFixup(root, hash string) (string, error) {
	stdout, stderr, err := run(root, []string{"commit", "--fixup=" + hash})
	return joinOutput(stdout, stderr), err
}

// RebaseAutosquash performs a non-interactive autosquash rebase from hash^ onward.
func RebaseAutosquash(root, hash string) (string, error) {
	return runWithExtraEnv(root,
		[]string{"rebase", "-i", "--autosquash", hash + "^"},
		[]string{"GIT_SEQUENCE_EDITOR=true"},
	)
}

// HasUnstagedChanges reports whether there are tracked working-tree changes not yet in the index.
func HasUnstagedChanges(root string) (bool, error) {
	_, _, err := run(root, []string{"diff", "--quiet"})
	if err != nil {
		if runErr, ok := err.(*RunError); ok && runErr.Code == 1 {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// StashPushAuto stashes all tracked working-tree changes plus untracked files.
func StashPushAuto(root string) (string, error) {
	stdout, stderr, err := run(root, []string{"stash", "push", "-u", "-m", "gx-amend-auto-stash"})
	return joinOutput(stdout, stderr), err
}

func runWithExtraEnv(dir string, args []string, extraEnv []string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(NonInteractiveEnv(), extraEnv...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	out := strings.TrimRight(outBuf.String()+errBuf.String(), "\r\n")
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return out, &RunError{
				Args:   args,
				Dir:    dir,
				Stdout: strings.TrimSpace(outBuf.String()),
				Stderr: strings.TrimSpace(errBuf.String()),
				Code:   ee.ExitCode(),
			}
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), runErr)
	}
	return out, nil
}
