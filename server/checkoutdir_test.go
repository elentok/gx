package server

import (
	"os/exec"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestCheckoutDirFor_ReturnsTheCheckoutOfTheTargetAndErrorsWhenThereIsNone(t *testing.T) {
	repo := testutil.TempRepo(t)
	if out, err := exec.Command("git", "-C", repo, "branch", "unchecked").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}

	_, dir, err := checkoutDirFor(repo, "main")
	if err != nil || dir != repo {
		t.Errorf("checkoutDirFor(main) = %q, %v; want %q", dir, err, repo)
	}
	if _, dir, err := checkoutDirFor(repo, "unchecked"); err == nil {
		t.Errorf("checkoutDirFor(unchecked) = %q, want an error", dir)
	}
}
