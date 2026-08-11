package git

import (
	"os"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree.
type Worktree struct {
	Path       string
	Name       string // path relative to the repo root
	Branch     string // short branch name, empty if detached
	Head       string // commit hash
	IsDetached bool
	IsBare     bool
}

// ListWorktrees returns all linked worktrees for the repo (excludes the bare root).
func ListWorktrees(repo Repo) ([]Worktree, error) {
	out, _, err := run(repo.Root, []string{"worktree", "list", "--porcelain"})
	if err != nil {
		return nil, err
	}
	return parseWorktreePorcelain(out, repo.LinkedWorktreeDir()), nil
}

func parseWorktreePorcelain(out, worktreeDir string) []Worktree {
	var worktrees []Worktree
	var cur *Worktree

	flush := func() {
		if cur != nil && !cur.IsBare {
			worktrees = append(worktrees, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		if after, ok := strings.CutPrefix(line, "worktree "); ok {
			flush()
			name := strings.TrimPrefix(after, worktreeDir+"/")
			cur = &Worktree{Path: after, Name: name}
		} else if cur == nil {
			continue
		} else if after, ok := strings.CutPrefix(line, "HEAD "); ok {
			cur.Head = after
		} else if after, ok := strings.CutPrefix(line, "branch "); ok {
			cur.Branch = strings.TrimPrefix(after, "refs/heads/")
		} else if line == "detached" {
			cur.IsDetached = true
		} else if line == "bare" {
			cur.IsBare = true
		}
	}
	flush()

	return worktrees
}

// RemoveWorktree removes a worktree by name or path.
func RemoveWorktree(repo Repo, name string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, _, err := run(repo.Root, args)
	return err
}

// MoveWorktree moves a worktree from one path to another.
func MoveWorktree(repo Repo, from, to string) error {
	_, _, err := run(repo.Root, []string{"worktree", "move", from, to})
	return err
}

// AddWorktree creates a new linked worktree at newPath with a new branch newName,
// starting at fromRef (branch name, tag, or commit hash). fromRef may be empty,
// in which case git uses the current HEAD of the repo.
func AddWorktree(repo Repo, newName, newPath, fromRef string) error {
	if err := excludeWorktreeDir(repo); err != nil {
		return err
	}
	args := []string{"worktree", "add", "-b", newName, newPath}
	if fromRef != "" {
		args = append(args, fromRef)
	}
	_, _, err := run(repo.Root, args)
	return err
}

// AddWorktreeOnBranch creates a linked worktree at newPath attached to the
// existing local branch branch (no -b, no new branch created) — the
// resume-after-park counterpart to AddWorktree's always-fresh-branch
// behavior, for reattaching to an iteration branch a prior park left intact.
func AddWorktreeOnBranch(repo Repo, branch, newPath string) error {
	if err := excludeWorktreeDir(repo); err != nil {
		return err
	}
	_, _, err := run(repo.Root, []string{"worktree", "add", newPath, branch})
	return err
}

// worktreeExcludeEntry computes the .git/info/exclude entry for repo's
// linked-worktree directory. It returns ok == false for bare repos, where
// linked worktrees live outside Root and need no exclusion, and for repos
// whose WorktreeDir isn't under Root at all.
func worktreeExcludeEntry(repo Repo) (entry string, ok bool) {
	if repo.IsBare {
		return "", false
	}
	wtDir := repo.LinkedWorktreeDir()
	rel, err := filepath.Rel(repo.Root, wtDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel) + "/", true
}

// isPathExcluded reports whether entry appears as an exact trimmed line in
// the exclude file at excludePath.
func isPathExcluded(excludePath, entry string) (bool, error) {
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return true, nil
		}
	}
	return false, nil
}

// excludeWorktreeDir makes sure the linked-worktree directory doesn't show up
// as untracked clutter in `git status` for the primary checkout, by appending
// an entry to .git/info/exclude (local-only, unlike .gitignore, so it isn't
// forced on the user via a committed file).
func excludeWorktreeDir(repo Repo) error {
	entry, ok := worktreeExcludeEntry(repo)
	if !ok {
		return nil
	}

	excludePath := filepath.Join(repo.Root, ".git", "info", "exclude")
	excluded, err := isPathExcluded(excludePath, entry)
	if err != nil {
		return err
	}
	if excluded {
		return nil
	}
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	_, err = f.WriteString(prefix + entry + "\n")
	return err
}
