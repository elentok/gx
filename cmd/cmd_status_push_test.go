package cmd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestExecute_RunsPush(t *testing.T) {
	for _, name := range []string{"push", "ps"} {
		t.Run(name, func(t *testing.T) {
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				getwd: func() (string, error) {
					return "/tmp", errors.New("boom")
				},
			}
			if err := execute([]string{name}, d); err == nil {
				t.Fatal("expected propagated error")
			}
		})
	}
}

func TestExecute_RunsStatus(t *testing.T) {
	for _, name := range []string{"status", "s"} {
		t.Run(name, func(t *testing.T) {
			called := 0
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(string) error {
					called++
					return nil
				},
			}

			if err := execute([]string{name}, d); err != nil {
				t.Fatalf("execute %s: %v", name, err)
			}
			if called != 1 {
				t.Fatalf("runStatus called %d times, want 1", called)
			}
		})
	}
}

func TestExecute_RunsStatusWithPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "relative", args: []string{"status", "README.md"}, want: "README.md"},
		{name: "alias", args: []string{"s", "/tmp/file.txt"}, want: "/tmp/file.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			d := deps{
				stdout: bytes.NewBuffer(nil),
				stderr: bytes.NewBuffer(nil),
				runStatus: func(path string) error {
					got = path
					return nil
				},
			}

			if err := execute(tc.args, d); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("runStatus path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveStatusTargetPath(t *testing.T) {
	root := "/repo"
	cwd := "/repo/sub"

	got, err := resolveStatusTargetPath(root, cwd, "file.txt")
	if err != nil {
		t.Fatalf("resolveStatusTargetPath relative: %v", err)
	}
	if got != "sub/file.txt" {
		t.Fatalf("relative target = %q, want %q", got, "sub/file.txt")
	}

	abs := filepath.Join(root, "deep", "file.txt")
	got, err = resolveStatusTargetPath(root, cwd, abs)
	if err != nil {
		t.Fatalf("resolveStatusTargetPath absolute: %v", err)
	}
	if got != "deep/file.txt" {
		t.Fatalf("absolute target = %q, want %q", got, "deep/file.txt")
	}
}

func TestResolveStatusTargetPathRejectsOutsideWorktree(t *testing.T) {
	_, err := resolveStatusTargetPath("/repo", "/repo", "../other.txt")
	if err == nil {
		t.Fatal("expected error for path outside worktree")
	}
}

func TestExecute_PushAllowedInRegularRepo(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	err := execute([]string{"push"}, d)
	if err == nil {
		t.Fatal("expected push failure in test repo without remote")
	}
	if strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("regular repo should be allowed, got: %v", err)
	}
}

func TestExecute_PushRejectedInBareRepo(t *testing.T) {
	repoDir := testutil.TempBareRepo(t)
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(string) (bool, error) { return false, nil },
	}

	err := execute([]string{"push"}, d)
	if err == nil {
		t.Fatal("expected error in bare repo")
	}
	if !strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_PushConfirmsBeforeCheckingDivergence(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	remote := t.TempDir() + "/remote.git"
	testutil.MustGitExported(t, ".", "clone", "--bare", repoDir, remote)
	testutil.MustGitExported(t, repoDir, "remote", "add", "origin", remote)

	prompts := []string{}
	d := deps{
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd: func() (string, error) {
			return repoDir, nil
		},
		confirmForce: func(prompt string) (bool, error) {
			prompts = append(prompts, prompt)
			return false, nil
		},
		choosePushDivergence: func(io.Reader, io.Writer, *git.PushDivergence) (int, error) {
			t.Fatalf("divergence chooser should not run before push confirmation")
			return 0, nil
		},
	}

	err := execute([]string{"push"}, d)
	if err == nil || err.Error() != "push aborted" {
		t.Fatalf("expected push aborted, got %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected exactly one confirmation prompt, got %v", prompts)
	}
	if prompts[0] != "Push branch main to origin?" {
		t.Fatalf("unexpected confirmation prompt: %q", prompts[0])
	}
}

func TestRunGitInteractive_Success(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	err := runGitInteractive(repoDir, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err != nil {
		t.Fatalf("runGitInteractive: %v", err)
	}
}

func TestRunGitInteractive_Failure(t *testing.T) {
	err := runGitInteractive(t.TempDir(), bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
}

func TestExecute_PushConfirmedNoRemote(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return true, nil
		},
	}
	err := execute([]string{"push"}, d)
	// Should fail at git fetch (no remote), not at confirm
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "push aborted" {
		t.Fatalf("expected to get past confirm step, got push aborted")
	}
}

func TestExecute_PushConfirmError(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	wantErr := errors.New("confirm failed")
	d := deps{
		stdin:  bytes.NewBuffer(nil),
		stdout: bytes.NewBuffer(nil),
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return repoDir, nil },
		confirmForce: func(string) (bool, error) {
			return false, wantErr
		},
	}
	err := execute([]string{"push"}, d)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestResolveStatusTargetPath_ExactRoot(t *testing.T) {
	_, err := resolveStatusTargetPath("/repo", "/repo", "/repo")
	if err == nil {
		t.Fatal("expected error for exact root target")
	}
}
