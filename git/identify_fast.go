package git

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// worktreeFields is what `git rev-parse --git-common-dir --show-toplevel`
// would print for a directory inside a worktree.
type worktreeFields struct {
	commonDir string
	topLevel  string
}

// riskyConfig matches settings that change where git finds the work tree.
var riskyConfig = regexp.MustCompile(`(?mi)^\s*(worktree\s*=|worktreeConfig\s*=)`)
var bareTrue = regexp.MustCompile(`(?mi)^\s*bare\s*=\s*(true|yes|on|1)\s*$`)

// fastWorktreeFields answers the common IdentifyDir question without forking
// git: it walks up from dir to the nearest `.git` (a directory, or a `gitdir:`
// file as in linked worktrees and the `.bare` layout). It reports ok=false
// whenever the layout is anything but plain (bare repos, GIT_* overrides,
// core.worktree, a cwd inside a git dir, unreadable files), and the caller
// then asks git itself.
func fastWorktreeFields(dir string) (worktreeFields, bool) {
	l, ok := discoverGit(dir)
	if !ok || l.bare {
		return worktreeFields{}, false
	}
	return fieldsFromLayout(l)
}

// gitLayout is where git keeps its files for a directory.
type gitLayout struct {
	cwd       string // dir with symlinks resolved
	top       string // the work tree's root; empty when cwd is inside a bare git dir
	gitDir    string // per-worktree: HEAD lives here
	commonDir string // shared: refs, packed-refs and config live here
	bare      bool   // cwd is inside the git dir itself
}

// discoverGit finds the git dirs for dir the way git's own upward search does,
// or reports ok=false for any layout it does not trust (GIT_* overrides,
// unreadable or odd files). It reads files only and forks nothing.
func discoverGit(dir string) (gitLayout, bool) {
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_NAMESPACE"} {
		if os.Getenv(k) != "" {
			return gitLayout{}, false
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return gitLayout{}, false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return gitLayout{}, false
	}
	for d := real; ; {
		gitPath := filepath.Join(d, ".git")
		fi, err := os.Lstat(gitPath)
		if err == nil {
			gitDir, commonDir, ok := resolveDotGit(d, gitPath, fi)
			if !ok {
				return gitLayout{}, false
			}
			return gitLayout{cwd: real, top: d, gitDir: gitDir, commonDir: commonDir}, true
		}
		if !os.IsNotExist(err) {
			return gitLayout{}, false
		}
		if looksLikeGitDir(d) {
			// Only a git dir that is the cwd itself is a bare repo; one found
			// further up means the cwd is inside it (hooks/, worktrees/x).
			if d == real {
				return gitLayout{cwd: real, gitDir: d, commonDir: d, bare: true}, true
			}
			return gitLayout{}, false
		}
		parent := filepath.Dir(d)
		if parent == d {
			return gitLayout{}, false
		}
		d = parent
	}
}

func looksLikeGitDir(d string) bool {
	if fi, err := os.Stat(filepath.Join(d, "HEAD")); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	for _, sub := range []string{"objects", "refs"} {
		if fi, err := os.Stat(filepath.Join(d, sub)); err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}

// resolveDotGit follows a `.git` directory or `gitdir:` file to the per-worktree
// git dir and the common dir.
func resolveDotGit(top, gitPath string, fi os.FileInfo) (gitDir, commonDir string, ok bool) {
	gitDir = gitPath
	switch {
	case fi.IsDir():
	case fi.Mode().IsRegular():
		b, err := os.ReadFile(gitPath)
		if err != nil {
			return "", "", false
		}
		line, found := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !found || strings.ContainsAny(line, "\r\n") {
			return "", "", false
		}
		gitDir = strings.TrimSpace(line)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(top, gitDir)
		}
	default:
		return "", "", false
	}
	gitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return "", "", false
	}

	commonDir = gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		c := strings.TrimSpace(string(b))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitDir, c)
		}
		if commonDir, err = filepath.EvalSymlinks(c); err != nil {
			return "", "", false
		}
	}
	// A linked worktree's own git dir has no objects/ or refs/; the common dir does.
	if !looksLikeGitDir(commonDir) {
		return "", "", false
	}
	return gitDir, commonDir, true
}

// fieldsFromLayout is what rev-parse would print for a worktree layout, or
// ok=false when the config could change that answer.
func fieldsFromLayout(l gitLayout) (worktreeFields, bool) {
	for _, cfg := range []string{filepath.Join(l.commonDir, "config"), filepath.Join(l.gitDir, "config")} {
		b, err := os.ReadFile(cfg)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return worktreeFields{}, false
		}
		// A bare common dir is fine for a linked worktree, but a main
		// "worktree" whose repo says bare=true is git's call to make.
		if riskyConfig.Match(b) || (l.commonDir == l.gitDir && bareTrue.Match(b)) {
			return worktreeFields{}, false
		}
	}

	out := l.commonDir
	if l.commonDir == filepath.Join(l.top, ".git") {
		// git prints this one relative to the cwd, and IdentifyDir resolves it
		// against dir as given (symlinks intact).
		rel, err := filepath.Rel(l.cwd, l.commonDir)
		if err != nil {
			return worktreeFields{}, false
		}
		out = rel
	}
	return worktreeFields{commonDir: out, topLevel: l.top}, true
}
