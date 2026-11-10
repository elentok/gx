package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func gitOut(t *testing.T, dir string, args ...string) (string, bool) {
	t.Helper()
	out, _, err := git.Run(dir, git.NonInteractiveEnv(), args...)
	return out, err == nil
}

// The no-fork ref lookup must agree with `git rev-parse --verify` wherever it
// claims an answer.
func TestResolveRevFast_MatchesGit(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	g := func(args ...string) { testutil.MustGitExported(t, repo, args...) }
	g("branch", "feature/x")
	g("branch", "packed")
	g("tag", "v1")
	g("tag", "same")
	g("branch", "same") // tag and branch with one name
	g("update-ref", "refs/remotes/origin/main", "HEAD")
	g("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	g("pack-refs", "--all", "--prune")
	g("branch", "loose-after-pack")
	testutil.WriteFile(t, repo, "f.txt", "x")
	testutil.CommitAll(t, repo, "second")

	_, wt := testutil.TempRepoWithLinkedWorktree(t, "wt")
	bareWT := testutil.TempBareRepoWithWorktrees(t, "feature")
	bare := testutil.TempBareRepo(t)

	revs := []string{
		"HEAD", "refs/heads/main", "refs/heads/feature/x", "refs/heads/packed",
		"refs/heads/loose-after-pack", "refs/heads/missing", "refs/heads/feature",
		"refs/tags/v1", "refs/remotes/origin/main", "refs/remotes/origin/HEAD",
		"main", "feature/x", "v1", "same", "origin/main", "origin", "missing", "deadbeef",
		"HEAD~1", "main^{commit}", "main..HEAD", "refs/heads/a b", "ORIG_HEAD",
	}
	for _, dir := range []string{repo, filepath.Join(repo, ".git"), wt, filepath.Join(bareWT, "feature"), bareWT, bare} {
		for _, rev := range revs {
			hash, found, ok := git.ResolveRevFast(dir, rev)
			if !ok {
				continue
			}
			want, wantOK := gitOut(t, dir, "rev-parse", "--verify", "--quiet", rev)
			if found != wantOK || (found && hash != want) {
				t.Errorf("dir %s rev %q: fast = (%q, %v), git = (%q, %v)", dir, rev, hash, found, want, wantOK)
			}
		}
	}

	// The common cases must be answered without git.
	for _, rev := range []string{"HEAD", "refs/heads/main", "refs/heads/packed", "refs/heads/missing", "main", "refs/remotes/origin/HEAD", "same"} {
		if _, _, ok := git.ResolveRevFast(repo, rev); !ok {
			t.Errorf("rev %q fell back to git", rev)
		}
	}
	for _, rev := range []string{"HEAD~1", "main^{commit}", "main..HEAD", "refs/heads/a b", "missing", "deadbeef"} {
		if _, _, ok := git.ResolveRevFast(repo, rev); ok {
			t.Errorf("rev %q was answered without git", rev)
		}
	}
}

func TestResolveRevFast_LinkedWorktreeHasItsOwnHEAD(t *testing.T) {
	t.Parallel()
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "wt")
	for _, dir := range []string{repo, wt} {
		hash, found, ok := git.ResolveRevFast(dir, "HEAD")
		want, _ := gitOut(t, dir, "rev-parse", "HEAD")
		if !ok || !found || hash != want {
			t.Errorf("%s: HEAD = (%q, %v, %v), want %q", dir, hash, found, ok, want)
		}
	}
	// Detach only the worktree: its HEAD must differ from the main one.
	testutil.MustGitExported(t, wt, "commit", "--allow-empty", "-m", "wt only")
	h1, _, _ := git.ResolveRevFast(repo, "HEAD")
	h2, _, _ := git.ResolveRevFast(wt, "HEAD")
	if h1 == h2 {
		t.Error("main and linked worktree report the same HEAD")
	}
}

func TestResolveRevFast_UnbornHEAD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.MustGitExported(t, dir, "init", "-q")
	if _, found, ok := git.ResolveRevFast(dir, "HEAD"); !ok || found {
		t.Errorf("unborn HEAD: found=%v ok=%v, want not found", found, ok)
	}
	if _, err := git.RevParse(dir, "HEAD"); !errors.Is(err, git.ErrRefNotFound) {
		t.Errorf("RevParse err = %v, want ErrRefNotFound", err)
	}
}

func TestCurrentBranchFast_MatchesGit(t *testing.T) {
	t.Parallel()
	repo := testutil.TempRepo(t)
	_, wt := testutil.TempRepoWithLinkedWorktree(t, "wt")
	check := func(label, dir string, wantFast bool) {
		t.Helper()
		got, ok := git.CurrentBranchFast(dir)
		if ok != wantFast {
			t.Errorf("%s: fast = %v, want %v", label, ok, wantFast)
		}
		if ok {
			if want, _ := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != want {
				t.Errorf("%s: %q, git says %q", label, got, want)
			}
		}
	}
	check("main", repo, true)
	check("linked worktree", wt, true)

	testutil.MustGitExported(t, repo, "tag", "main") // makes git print heads/main
	check("tag shadows branch", repo, false)

	testutil.MustGitExported(t, repo, "checkout", "--detach")
	check("detached", repo, true)

	empty := t.TempDir()
	testutil.MustGitExported(t, empty, "init", "-q")
	check("unborn", empty, false)

	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check("garbage HEAD", repo, false)
}
