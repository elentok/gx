package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestExecute_ListWorktrees(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := execute([]string{"wt", "list"}, d); err != nil {
		t.Fatalf("execute wt list: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 worktrees, got %d: %v", len(lines), lines)
	}
	if lines[0] != "feature-a" || lines[1] != "feature-b" {
		t.Fatalf("unexpected worktree names: %v", lines)
	}
}

func TestExecute_WorktreeAbsPath(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := execute([]string{"wt", "abs-path", "feature-a"}, d); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-a"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_ListWorktrees_FromInsideWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := execute([]string{"wt", "list"}, d); err != nil {
		t.Fatalf("execute wt list: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.ContainsRune(line, '/') {
			t.Errorf("wt list output contains path separator: %q", line)
		}
	}
}

func TestExecute_WorktreeAbsPath_FromInsideWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := execute([]string{"wt", "abs-path", "feature-b"}, d); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-b"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_WorktreeAbsPath_NotFound(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
	}

	err := execute([]string{"wt", "abs-path", "does-not-exist"}, d)
	if err == nil {
		t.Fatal("expected error for missing worktree")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_WorktreeAbsPath_MissingArg(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}

	err := execute([]string{"wt", "abs-path"}, d)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestExecute_WtClone_NoArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"wt", "clone"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_TooManyArgs(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
	}
	err := execute([]string{"wt", "clone", "a", "b", "c"}, d)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_GetWdError(t *testing.T) {
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "", errors.New("no dir") },
	}
	err := execute([]string{"wt", "clone", "https://example.com/repo.git"}, d)
	if err == nil || !strings.Contains(err.Error(), "no dir") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}
