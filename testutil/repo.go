package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// evalDir resolves symlinks in a directory path. On macOS, t.TempDir() returns
// /var/... which is a symlink to /private/var/..., while git resolves the real path.
func evalDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}
	return real
}

// TempRepo creates a regular git repo with one initial commit on "main".
func TempRepo(t *testing.T) string {
	t.Helper()
	dir := evalDir(t, t.TempDir())
	mustGit(t, dir, "init", "--initial-branch=main")
	configUser(t, dir)
	WriteFile(t, dir, "README.md", "# test")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "initial")
	return dir
}

// TempBareRepo creates a bare git repo by cloning a regular repo.
// The bare repo has one commit on "main", with remote tracking refs populated
// and main configured to track origin/main.
func TempBareRepo(t *testing.T) string {
	t.Helper()
	src := TempRepo(t)
	bare := evalDir(t, t.TempDir())
	// Remove the empty TempDir so git clone can create it cleanly
	os.RemoveAll(bare)
	// Register a cleanup that removes the repo before t.TempDir's cleanup runs
	// (t.Cleanup is LIFO). Retrying handles any lingering background git processes
	// or macOS APFS races that cause os.RemoveAll to return ENOTEMPTY.
	t.Cleanup(func() {
		for range 10 {
			if os.RemoveAll(bare) == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	mustRun(t, ".", "git", "clone", "--bare", src, bare)
	// Configure origin to populate refs/remotes/origin/* on fetch (bare clones
	// use refs/heads/* by default), then fetch so remote tracking refs exist.
	mustGit(t, bare, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	mustGit(t, bare, "fetch", "origin")
	mustGit(t, bare, "branch", "--set-upstream-to=origin/main", "main")
	return bare
}

// TempBareRepoWithMainWorktreeAhead creates a bare repo where local main is one
// commit behind origin/main. The repo contains a linked worktree for main
// (at the old tip) plus a linked worktree for each name in featureNames.
//
// This lets tests verify that the view refreshes after pulling main: before the
// pull feature branches show base-status ✓ (rebased on old main); after the
// pull, main advances and they show ✗.
func TempBareRepoWithMainWorktreeAhead(t *testing.T, featureNames ...string) string {
	t.Helper()
	src := TempRepo(t)

	// Add a second commit to src so origin/main is ahead of the first commit.
	WriteFile(t, src, "v2.txt", "second commit")
	mustGit(t, src, "add", ".")
	mustGit(t, src, "commit", "-m", "second commit on main")

	// Clone src as a bare repo (acquires both commits; local main = C2 = origin/main).
	bare := evalDir(t, t.TempDir())
	os.RemoveAll(bare)
	t.Cleanup(func() {
		for range 10 {
			if os.RemoveAll(bare) == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	mustRun(t, ".", "git", "clone", "--bare", src, bare)
	mustGit(t, bare, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	mustGit(t, bare, "fetch", "origin")
	mustGit(t, bare, "branch", "--set-upstream-to=origin/main", "main")

	// Reset local main back to C1 so the repo has something to pull.
	mustGit(t, bare, "update-ref", "refs/heads/main", "HEAD~1")

	// Add a linked worktree for main (at C1).
	mainWt := filepath.Join(bare, "main")
	mustGit(t, bare, "worktree", "add", mainWt, "main")
	configUser(t, mainWt)

	// Add feature worktrees branched from C1.
	for _, name := range featureNames {
		wtDir := filepath.Join(bare, name)
		mustGit(t, bare, "worktree", "add", "-b", name, wtDir)
		configUser(t, wtDir)
		WriteFile(t, wtDir, "file.txt", name)
		mustGit(t, wtDir, "add", ".")
		mustGit(t, wtDir, "commit", "-m", "add "+name)
	}
	return bare
}

// TempBareRepoWithWorktrees creates a bare repo with linked worktrees.
// Each name results in a branch and a worktree directory under the bare repo.
func TempBareRepoWithWorktrees(t *testing.T, names ...string) string {
	t.Helper()
	repoDir := TempBareRepo(t)
	for _, name := range names {
		wtDir := filepath.Join(repoDir, name)
		mustGit(t, repoDir, "worktree", "add", "-b", name, wtDir)
		configUser(t, wtDir)
		WriteFile(t, wtDir, "file.txt", name)
		mustGit(t, wtDir, "add", ".")
		mustGit(t, wtDir, "commit", "-m", "add "+name)
	}
	return repoDir
}

// TempDotBareRepoWithWorktrees creates a .bare-style repo layout:
//
//	outer/
//	  .bare/   ← actual bare git repo
//	  .git      ← "gitdir: ./.bare"
//	  <name>/  ← linked worktrees
//
// Returns the outer directory path.
func TempDotBareRepoWithWorktrees(t *testing.T, names ...string) string {
	t.Helper()
	src := TempRepo(t)
	outer := evalDir(t, t.TempDir())
	// Remove the outer repo before t.TempDir cleanup (LIFO) and retry to absorb
	// transient macOS/APFS ENOTEMPTY races from lingering git filesystem activity.
	t.Cleanup(func() {
		for range 10 {
			if os.RemoveAll(outer) == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	bareDir := filepath.Join(outer, ".bare")
	mustRun(t, ".", "git", "clone", "--bare", src, bareDir)
	mustGit(t, bareDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	mustGit(t, bareDir, "fetch", "origin")
	mustGit(t, bareDir, "branch", "--set-upstream-to=origin/main", "main")

	// Write the .git pointer file so git recognises outer as the repo root.
	if err := os.WriteFile(filepath.Join(outer, ".git"), []byte("gitdir: ./.bare\n"), 0644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	for _, name := range names {
		wtDir := filepath.Join(outer, name)
		mustGit(t, bareDir, "worktree", "add", "-b", name, wtDir)
		configUser(t, wtDir)
		WriteFile(t, wtDir, "file.txt", name)
		mustGit(t, wtDir, "add", ".")
		mustGit(t, wtDir, "commit", "-m", "add "+name)
	}
	return outer
}

// WriteFile writes content to a file inside dir, creating it if needed.
func WriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func configUser(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")
}

// Mkdir creates a directory, failing the test if it can't.
func Mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("Mkdir %s: %v", path, err)
	}
}

// CommitAll stages all changes in dir and creates a commit with the given message.
func CommitAll(t *testing.T, dir, message string) {
	t.Helper()
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", message)
}

// SetBranchUpstream sets the upstream tracking reference for a local branch.
func SetBranchUpstream(t *testing.T, dir, branch, upstream string) {
	t.Helper()
	mustGit(t, dir, "branch", "--set-upstream-to="+upstream, branch)
}

// PushBranchWithUpstream pushes branch to remote and sets the upstream tracking ref.
func PushBranchWithUpstream(t *testing.T, dir, remote, branch string) {
	t.Helper()
	mustGit(t, dir, "push", "--set-upstream", remote, branch)
}

// AmendLastCommit adds a marker file and amends the last commit, changing its hash.
func AmendLastCommit(t *testing.T, dir string) {
	t.Helper()
	WriteFile(t, dir, ".amend-marker", "amended")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "--amend", "--no-edit")
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	mustRun(t, dir, "git", args...)
}

// MustGitExported runs a git command in dir, failing the test on error.
// It is the exported equivalent of mustGit for use in other packages.
func MustGitExported(t *testing.T, dir string, args ...string) {
	t.Helper()
	mustGit(t, dir, args...)
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v in %s: %v\n%s", name, args, dir, err, out)
	}
}
