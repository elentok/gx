package git

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestGitDirs_PlainCheckoutHasOneDir(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	gitDir, commonDir, err := GitDirs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gitDir != commonDir {
		t.Errorf("gitDir = %q, commonDir = %q, want equal", gitDir, commonDir)
	}
	if filepath.Base(commonDir) != ".git" || !filepath.IsAbs(commonDir) {
		t.Errorf("commonDir = %q, want an absolute .git", commonDir)
	}
}

func TestGitDirs_LinkedWorktreeGitDirIsUnderCommonDir(t *testing.T) {
	t.Parallel()
	_, wt := testutil.TempRepoWithLinkedWorktree(t, "feat")
	gitDir, commonDir, err := GitDirs(wt)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(commonDir, "worktrees", "feat"); gitDir != want {
		t.Errorf("gitDir = %q, want %q", gitDir, want)
	}
	if !filepath.IsAbs(commonDir) {
		t.Errorf("commonDir = %q, want absolute", commonDir)
	}
}

func TestGitDirs_BareRepoWithWorktrees(t *testing.T) {
	t.Parallel()
	outer := testutil.TempDotBareRepoWithWorktrees(t, "feat")
	gitDir, commonDir, err := GitDirs(filepath.Join(outer, "feat"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(commonDir, "worktrees", "feat"); gitDir != want {
		t.Errorf("gitDir = %q, want %q", gitDir, want)
	}
	if filepath.Base(commonDir) != ".bare" {
		t.Errorf("commonDir = %q, want the .bare dir", commonDir)
	}
}

func TestGitDirs_NotARepo(t *testing.T) {
	t.Parallel()
	if _, _, err := GitDirs(t.TempDir()); err == nil {
		t.Error("error = nil, want ErrNoRepo")
	}
}
