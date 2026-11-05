package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/cmd/gitops"
	"github.com/elentok/gx/testutil"
)

// runWT runs args against the whole `worktrees` tree under env.
func runWT(t *testing.T, env cmdbase.Env, args []string) error {
	t.Helper()
	d := Deps{Env: env}
	return cmdtest.Run(t, env, args, NewWorktrees(d, gitops.WorktreeCommands(gitops.Deps{Env: env})...))
}

func TestExecute_ListWorktrees(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	var stdout bytes.Buffer
	env := cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := runWT(t, env, []string{"wt", "list"}); err != nil {
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
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	var stdout bytes.Buffer
	env := cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return repoDir, nil },
	}

	if err := runWT(t, env, []string{"wt", "abs-path", "feature-a"}); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-a"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_ListWorktrees_FromInsideWorktree(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	env := cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := runWT(t, env, []string{"wt", "list"}); err != nil {
		t.Fatalf("execute wt list: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.ContainsRune(line, '/') {
			t.Errorf("wt list output contains path separator: %q", line)
		}
	}
}

func TestExecute_WorktreeAbsPath_FromInsideWorktree(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	wtDir := repoDir + "/feature-a"
	var stdout bytes.Buffer
	env := cmdbase.Env{
		Stdout: &stdout,
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return wtDir, nil },
	}

	if err := runWT(t, env, []string{"wt", "abs-path", "feature-b"}); err != nil {
		t.Fatalf("execute wt abs-path: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := repoDir + "/feature-b"
	if got != want {
		t.Fatalf("abs path = %q, want %q", got, want)
	}
}

func TestExecute_WorktreeAbsPath_NotFound(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	env := cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return repoDir, nil },
	}

	err := runWT(t, env, []string{"wt", "abs-path", "does-not-exist"})
	if err == nil {
		t.Fatal("expected error for missing worktree")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_WorktreeAbsPath_MissingArg(t *testing.T) {
	t.Parallel()
	env := cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
	}

	err := runWT(t, env, []string{"wt", "abs-path"})
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestExecute_WtClone_NoArgs(t *testing.T) {
	t.Parallel()
	env := cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
	}
	err := runWT(t, env, []string{"wt", "clone"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_TooManyArgs(t *testing.T) {
	t.Parallel()
	env := cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
	}
	err := runWT(t, env, []string{"wt", "clone", "a", "b", "c"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

func TestExecute_WtClone_GetWdError(t *testing.T) {
	t.Parallel()
	env := cmdbase.Env{
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		Getwd:  func() (string, error) { return "", errors.New("no dir") },
	}
	err := runWT(t, env, []string{"wt", "clone", "https://example.com/repo.git"})
	if err == nil || !strings.Contains(err.Error(), "no dir") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}
