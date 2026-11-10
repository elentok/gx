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
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM"} {
		if os.Getenv(k) != "" {
			return worktreeFields{}, false
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return worktreeFields{}, false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return worktreeFields{}, false
	}
	for d := real; ; {
		if looksLikeGitDir(d) {
			return worktreeFields{}, false
		}
		gitPath := filepath.Join(d, ".git")
		fi, err := os.Lstat(gitPath)
		if err == nil {
			return fieldsFromDotGit(real, d, gitPath, fi)
		}
		if !os.IsNotExist(err) {
			return worktreeFields{}, false
		}
		parent := filepath.Dir(d)
		if parent == d {
			return worktreeFields{}, false
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

func fieldsFromDotGit(cwd, top, gitPath string, fi os.FileInfo) (worktreeFields, bool) {
	gitDir := gitPath
	switch {
	case fi.IsDir():
	case fi.Mode().IsRegular():
		b, err := os.ReadFile(gitPath)
		if err != nil {
			return worktreeFields{}, false
		}
		line, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok || strings.ContainsAny(line, "\r\n") {
			return worktreeFields{}, false
		}
		gitDir = strings.TrimSpace(line)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(top, gitDir)
		}
	default:
		return worktreeFields{}, false
	}
	gitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return worktreeFields{}, false
	}

	commonDir := gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		c := strings.TrimSpace(string(b))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitDir, c)
		}
		if commonDir, err = filepath.EvalSymlinks(c); err != nil {
			return worktreeFields{}, false
		}
	}
	// A linked worktree's own git dir has no objects/ or refs/; the common dir does.
	if !looksLikeGitDir(commonDir) {
		return worktreeFields{}, false
	}
	for _, cfg := range []string{filepath.Join(commonDir, "config"), filepath.Join(gitDir, "config")} {
		b, err := os.ReadFile(cfg)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return worktreeFields{}, false
		}
		// A bare common dir is fine for a linked worktree, but a main
		// "worktree" whose repo says bare=true is git's call to make.
		if riskyConfig.Match(b) || (commonDir == gitDir && bareTrue.Match(b)) {
			return worktreeFields{}, false
		}
	}

	out := commonDir
	if commonDir == filepath.Join(top, ".git") {
		// git prints this one relative to the cwd, and IdentifyDir resolves it
		// against dir as given (symlinks intact).
		rel, err := filepath.Rel(cwd, commonDir)
		if err != nil {
			return worktreeFields{}, false
		}
		out = rel
	}
	return worktreeFields{commonDir: out, topLevel: top}, true
}
