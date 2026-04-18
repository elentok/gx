package git_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestDetectPushDivergence_NoUpstream(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	wtDir := filepath.Join(repoDir, "feature-a")

	div, err := git.DetectPushDivergence(wtDir, "feature-a")
	if err != nil {
		t.Fatalf("DetectPushDivergence: %v", err)
	}
	if div != nil {
		t.Fatalf("expected no divergence without upstream, got %+v", *div)
	}
}

func TestDetectPushDivergence_Diverged(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	wtDir := filepath.Join(repoDir, "feature-a")

	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature-a")
	testutil.AmendLastCommit(t, wtDir)

	div, err := git.DetectPushDivergence(wtDir, "feature-a")
	if err != nil {
		t.Fatalf("DetectPushDivergence: %v", err)
	}
	if div == nil {
		t.Fatalf("expected divergence, got nil")
	}
	if div.Branch != "feature-a" {
		t.Fatalf("branch = %q, want feature-a", div.Branch)
	}
	if div.Local.Hash == "" || div.RemoteHead.Hash == "" {
		t.Fatalf("expected commit hashes, got local=%q remote=%q", div.Local.Hash, div.RemoteHead.Hash)
	}
	if div.Local.Date.IsZero() || div.RemoteHead.Date.IsZero() {
		t.Fatalf("expected non-zero commit dates, got local=%v remote=%v", div.Local.Date, div.RemoteHead.Date)
	}
}
