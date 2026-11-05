package gitops

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

// pushDeps builds Deps for `gx push` run in dir; ConfirmForce is the answer
// every prompt gets.
func pushDeps(t *testing.T, dir string, confirm func(string) (bool, error)) Deps {
	t.Helper()
	return Deps{
		Env: cmdbase.Env{
			Stdin:  bytes.NewBuffer(nil),
			Stdout: bytes.NewBuffer(nil),
			Stderr: bytes.NewBuffer(nil),
			Getwd:  func() (string, error) { return dir, nil },
			Env:    cmdtest.EnvFor(t),
		},
		ConfirmForce: confirm,
	}
}

func runPushCmd(t *testing.T, d Deps) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, []string{"push"}, NewPush(d))
}

func TestExecute_RunsPush(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"push", "ps"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := pushDeps(t, "", nil)
			d.Getwd = func() (string, error) {
				return "/tmp", errors.New("boom")
			}
			if err := cmdtest.Run(t, d.Env, []string{name}, NewPush(d)); err == nil {
				t.Fatal("expected propagated error")
			}
		})
	}
}

func TestExecute_PushAllowedInRegularRepo(t *testing.T) {
	t.Parallel()
	d := pushDeps(t, testutil.TempRepo(t), func(string) (bool, error) { return false, nil })

	err := runPushCmd(t, d)
	if err == nil {
		t.Fatal("expected push failure in test repo without remote")
	}
	if strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("regular repo should be allowed, got: %v", err)
	}
}

func TestExecute_PushRejectedInBareRepo(t *testing.T) {
	t.Parallel()
	d := pushDeps(t, testutil.TempBareRepo(t), func(string) (bool, error) { return false, nil })

	err := runPushCmd(t, d)
	if err == nil {
		t.Fatal("expected error in bare repo")
	}
	if !strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_PushConfirmsBeforeCheckingDivergence(t *testing.T) {
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	remote := t.TempDir() + "/remote.git"
	testutil.MustGitExported(t, ".", "clone", "--bare", repoDir, remote)
	testutil.MustGitExported(t, repoDir, "remote", "add", "origin", remote)

	prompts := []string{}
	d := pushDeps(t, repoDir, func(prompt string) (bool, error) {
		prompts = append(prompts, prompt)
		return false, nil
	})
	d.ChoosePushDivergence = func(io.Reader, io.Writer, *git.PushDivergence) (int, error) {
		t.Fatalf("divergence chooser should not run before push confirmation")
		return 0, nil
	}

	err := runPushCmd(t, d)
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
	t.Parallel()
	repoDir := testutil.TempRepo(t)
	err := runGitInteractive(repoDir, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err != nil {
		t.Fatalf("runGitInteractive: %v", err)
	}
}

func TestRunGitInteractive_Failure(t *testing.T) {
	t.Parallel()
	err := runGitInteractive(t.TempDir(), bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil), "status")
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
}

func TestExecute_PushConfirmedNoRemote(t *testing.T) {
	t.Parallel()
	d := pushDeps(t, testutil.TempRepo(t), func(string) (bool, error) { return true, nil })
	err := runPushCmd(t, d)
	// Should fail at git fetch (no remote), not at confirm
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "push aborted" {
		t.Fatalf("expected to get past confirm step, got push aborted")
	}
}

func TestExecute_PushConfirmError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("confirm failed")
	d := pushDeps(t, testutil.TempRepo(t), func(string) (bool, error) { return false, wantErr })
	err := runPushCmd(t, d)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
