package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestRunStashify_NoArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}
	err := runStashify(nil, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestRunStashify_NoChanges_RunsCommand(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	var stdout bytes.Buffer
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	marker := filepath.Join(repoDir, "marker.txt")
	err := runStashify([]string{"touch", marker}, d)
	if err != nil {
		t.Fatalf("runStashify: %v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatal("expected command to run (marker file not created)")
	}
}

func TestRunStashify_WithChanges_StashesRunsUnstashes(t *testing.T) {
	repoDir := testutil.TempRepo(t)

	// Modify a tracked file to create a stashable change.
	testutil.WriteFile(t, repoDir, "README.md", "modified")

	var stdout, stderr bytes.Buffer
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return repoDir, nil },
	}

	marker := filepath.Join(repoDir, "marker.txt")
	err := runStashify([]string{"touch", marker}, d)
	if err != nil {
		t.Fatalf("runStashify: %v", err)
	}

	// marker was created by the command
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatal("expected marker file to exist")
	}

	// stash should be empty after successful pop
	out, gitErr := exec.Command("git", "-C", repoDir, "stash", "list").Output()
	if gitErr != nil {
		t.Fatalf("git stash list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected stash to be empty after pop, got: %s", out)
	}

	// original change should be restored
	content, readErr := os.ReadFile(filepath.Join(repoDir, "README.md"))
	if readErr != nil {
		t.Fatalf("reading README.md: %v", readErr)
	}
	if string(content) != "modified" {
		t.Fatalf("README.md = %q, want %q", string(content), "modified")
	}
}

func TestRunStashify_WithChanges_CommandFails_UserConfirmsPop(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	// Modify a tracked file so git stash actually stashes it.
	testutil.WriteFile(t, repoDir, "README.md", "modified for confirm test")

	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return true, nil
		},
	}

	err := runStashify([]string{"false"}, d)
	if err == nil {
		t.Fatal("expected command error")
	}

	// stash should be popped
	out, gitErr := exec.Command("git", "-C", repoDir, "stash", "list").Output()
	if gitErr != nil {
		t.Fatalf("git stash list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected empty stash after confirmed pop, got: %s", out)
	}

	// README.md restored to modified content
	content, readErr := os.ReadFile(filepath.Join(repoDir, "README.md"))
	if readErr != nil {
		t.Fatalf("reading README.md: %v", readErr)
	}
	if string(content) != "modified for confirm test" {
		t.Fatalf("README.md = %q, want modified for confirm test", string(content))
	}
}

func TestRunStashify_WithChanges_CommandFails_UserDeclinesPop(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	// Modify a tracked file so git stash actually stashes it.
	testutil.WriteFile(t, repoDir, "README.md", "modified for decline test")

	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return false, nil
		},
	}

	err := runStashify([]string{"false"}, d)
	if err == nil {
		t.Fatal("expected command error")
	}

	// stash should still have 1 entry
	out, gitErr := exec.Command("git", "-C", repoDir, "stash", "list").Output()
	if gitErr != nil {
		t.Fatalf("git stash list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("expected stash to remain when user declines pop")
	}

	// README.md should be back to original (stashed content not popped)
	content, readErr := os.ReadFile(filepath.Join(repoDir, "README.md"))
	if readErr != nil {
		t.Fatalf("reading README.md: %v", readErr)
	}
	if string(content) != "# test" {
		t.Fatalf("README.md = %q, expected original content when stash not popped", string(content))
	}
}

func TestRunStashify_GetWdError(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "", errors.New("no dir") },
	}
	err := runStashify([]string{"true"}, d)
	if err == nil || !strings.Contains(err.Error(), "no dir") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}
