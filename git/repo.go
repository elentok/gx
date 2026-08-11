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
		return identifyWorktree(dir)
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

func identifyWorktree(dir string) (*DirInfo, error) {
	topLevel := runAllowFail(dir, []string{"rev-parse", "--show-toplevel"})
	if topLevel == "" {
		return nil, fmt.Errorf("inside worktree at %q but --show-toplevel failed", dir)
	}

	// --git-common-dir (unlike --git-dir) resolves to the shared .git dir
	// even from inside a linked worktree, where --git-dir instead points at
	// .git/worktrees/<name>. Using --git-dir here would misclassify a linked
	// worktree of a regular repo as belonging to a bare repo.
	commonDir := runAllowFail(dir, []string{"rev-parse", "--git-common-dir"})
	if commonDir == "" {
		return nil, fmt.Errorf("cannot resolve git common dir for %q", dir)
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	commonDir = filepath.Clean(commonDir)

	if filepath.Base(commonDir) == ".git" {
		// Regular (non-bare) repository. Linked worktrees live under
		// <Root>/.worktrees/ instead of directly under Root, so they don't
		// clutter the primary checkout's own file listing.
		root := filepath.Dir(commonDir)
		worktreeDir := filepath.Join(root, ".worktrees")
		info := &DirInfo{
			Repo:           Repo{Root: root, WorktreeDir: worktreeDir, IsBare: false, MainBranch: detectMainBranch(root)},
			IsRepoRoot:     root == dir,
			IsWorktreeRoot: topLevel == dir,
		}
		if topLevel != root {
			info.WorktreeRoot = topLevel
		}
		return info, nil
	}

	// Linked worktree inside a bare repo - commonDir is the bare repo root itself.
	repoRoot := commonDir

	// For the .bare trick, worktrees live in the parent directory alongside .bare/.
	worktreeDir := repoRoot
	if filepath.Base(repoRoot) == ".bare" {
		worktreeDir = filepath.Dir(repoRoot)
	}

	return &DirInfo{
		Repo:           Repo{Root: repoRoot, WorktreeDir: worktreeDir, IsBare: true, MainBranch: detectMainBranch(repoRoot)},
		WorktreeRoot:   topLevel,
		IsRepoRoot:     repoRoot == dir,
		IsWorktreeRoot: topLevel == dir,
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
