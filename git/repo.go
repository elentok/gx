package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Repo represents a git repository.
type Repo struct {
	Root        string
	WorktreeDir string // directory where linked worktrees live; if empty, defaults to Root
	IsBare      bool
	MainBranch  string // "main" or "master"
}

// LinkedWorktreeDir returns the directory where linked worktrees are created.
// For normal bare repos this is the same as Root; for .bare repos it is the
// parent directory that contains both .bare/ and the worktrees.
func (r Repo) LinkedWorktreeDir() string {
	if r.WorktreeDir != "" {
		return r.WorktreeDir
	}
	return r.Root
}

// ScratchRoot returns the single canonical location of the local ticket
// tracker's `.scratch` directory: Root/.scratch. For a bare-repo checkout
// this is the bare git directory's own `.scratch`, not any linked worktree's.
func (r Repo) ScratchRoot() string {
	return filepath.Join(r.Root, ".scratch")
}

// DirInfo describes what kind of git context a directory is in.
type DirInfo struct {
	Repo           Repo
	WorktreeRoot   string // non-empty when inside a linked worktree
	IsRepoRoot     bool
	IsWorktreeRoot bool
}

// FindRepo walks up from dir to find a git repository.
func FindRepo(dir string) (*Repo, error) {
	info, err := IdentifyDir(dir)
	if err != nil {
		return nil, err
	}
	return &info.Repo, nil
}

// IdentifyDir returns context about the directory: which repo it belongs to,
// whether it's a worktree root, etc.
func IdentifyDir(dir string) (*DirInfo, error) {
	gitDir := runAllowFail(dir, []string{"rev-parse", "--git-dir"})
	if gitDir == "" {
		return nil, fmt.Errorf("no git repo found at %q", dir)
	}

	isInsideWorktree := runAllowFail(dir, []string{"rev-parse", "--is-inside-work-tree"}) == "true"

	if isInsideWorktree {
		return identifyWorktree(dir, gitDir)
	}

	// Bare repo: gitDir "." means the current dir is the git dir itself.
	// Otherwise resolve to an absolute path (git may return a relative path,
	// e.g. ".bare" for the .bare trick).
	repoRoot := dir
	if gitDir != "." {
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
		repoRoot = gitDir
	}

	// For the .bare trick, worktrees live in the parent directory alongside .bare/.
	worktreeDir := repoRoot
	if filepath.Base(repoRoot) == ".bare" {
		worktreeDir = filepath.Dir(repoRoot)
	}

	return &DirInfo{
		Repo:           Repo{Root: repoRoot, WorktreeDir: worktreeDir, IsBare: true, MainBranch: detectMainBranch(repoRoot)},
		IsRepoRoot:     repoRoot == dir || worktreeDir == dir,
		IsWorktreeRoot: false,
	}, nil
}

func identifyWorktree(dir, gitDir string) (*DirInfo, error) {
	topLevel := runAllowFail(dir, []string{"rev-parse", "--show-toplevel"})
	if topLevel == "" {
		return nil, fmt.Errorf("inside worktree at %q but --show-toplevel failed", dir)
	}

	gitDirName := filepath.Base(gitDir)

	if gitDirName == ".git" {
		// Regular (non-bare) repository. Linked worktrees live under
		// <Root>/.worktrees/ instead of directly under Root, so they don't
		// clutter the primary checkout's own file listing.
		worktreeDir := filepath.Join(topLevel, ".worktrees")
		return &DirInfo{
			Repo:           Repo{Root: topLevel, WorktreeDir: worktreeDir, IsBare: false, MainBranch: detectMainBranch(topLevel)},
			IsRepoRoot:     topLevel == dir,
			IsWorktreeRoot: topLevel == dir,
		}, nil
	}

	// Linked worktree inside a bare repo - find the bare repo root one level up
	worktreeRoot := topLevel
	parentDir := filepath.Dir(worktreeRoot)
	parentGitDir := runAllowFail(parentDir, []string{"rev-parse", "--git-dir"})
	if parentGitDir == "" {
		return nil, fmt.Errorf("cannot find bare repo root for worktree %q", worktreeRoot)
	}

	repoRoot := parentDir
	if parentGitDir != "." {
		if !filepath.IsAbs(parentGitDir) {
			parentGitDir = filepath.Join(parentDir, parentGitDir)
		}
		repoRoot = parentGitDir
	}

	// For the .bare trick, worktrees live in the parent directory alongside .bare/.
	worktreeDir := repoRoot
	if filepath.Base(repoRoot) == ".bare" {
		worktreeDir = filepath.Dir(repoRoot)
	}

	return &DirInfo{
		Repo:           Repo{Root: repoRoot, WorktreeDir: worktreeDir, IsBare: true, MainBranch: detectMainBranch(repoRoot)},
		WorktreeRoot:   worktreeRoot,
		IsRepoRoot:     repoRoot == dir,
		IsWorktreeRoot: worktreeRoot == dir,
	}, nil
}

// detectMainBranch returns "main" or "master" depending on what exists in the repo.
func detectMainBranch(repoRoot string) string {
	return RemoteDefaultBranch(repoRoot)
}

// RemoteDefaultBranch returns the repository's default branch using origin/HEAD
// when available, then falls back to local branch checks.
func RemoteDefaultBranch(repoRoot string) string {
	// Check origin/HEAD first (most reliable for cloned repos)
	out := runAllowFail(repoRoot, []string{"symbolic-ref", "--short", "refs/remotes/origin/HEAD"})
	if out != "" {
		// out is like "origin/main" - strip the remote prefix
		if _, after, ok := strings.Cut(out, "/"); ok {
			return after
		}
	}

	// Fall back to checking local branches
	if runAllowFail(repoRoot, []string{"rev-parse", "--verify", "refs/heads/main"}) != "" {
		return "main"
	}
	if runAllowFail(repoRoot, []string{"rev-parse", "--verify", "refs/heads/master"}) != "" {
		return "master"
	}

	return "main"
}
