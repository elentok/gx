package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Issue represents a problem found by the doctor check.
type Issue struct {
	Description    string
	FixDescription string
	fixFn          func() error
}

// CanFix reports whether an automatic fix is available for this issue.
func (i Issue) CanFix() bool { return i.fixFn != nil }

// Fix applies the automatic fix.
func (i Issue) Fix() error { return i.fixFn() }

// CheckRepo runs all health checks on repo and returns any issues found.
func CheckRepo(repo Repo) ([]Issue, error) {
	var issues []Issue

	// Check 1: origin fetch refspec
	if problem := CheckFetchConfig(repo.Root); problem != nil {
		issues = append(issues, Issue{
			Description:    problem.Description,
			FixDescription: "Fix fetch refspec and run git fetch",
			fixFn:          func() error { return FixFetchConfig(repo.Root) },
		})
	}

	// Checks 2 & 3: .bare trick
	if filepath.Base(repo.Root) == ".bare" {
		dotBareIssues, err := checkDotBare(repo)
		if err != nil {
			return nil, err
		}
		issues = append(issues, dotBareIssues...)
	}

	// Check 4: standard (non-bare) clones exclude .worktrees/ from git status
	if !repo.IsBare {
		if issue := checkWorktreesExclude(repo); issue != nil {
			issues = append(issues, *issue)
		}
	}

	return issues, nil
}

// checkWorktreesExclude reports whether the linked-worktree directory for a
// standard clone is listed in .git/info/exclude, mirroring the .bare-trick
// checks above. It only fires once .worktrees/ actually exists - an unused
// repo that never created a worktree has nothing to hide from git status.
func checkWorktreesExclude(repo Repo) *Issue {
	wtDir := repo.LinkedWorktreeDir()
	rel, err := filepath.Rel(repo.Root, wtDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil
	}
	if _, err := os.Stat(wtDir); os.IsNotExist(err) {
		return nil
	}
	entry := filepath.ToSlash(rel) + "/"

	excludePath := filepath.Join(repo.Root, ".git", "info", "exclude")
	data, _ := os.ReadFile(excludePath)
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	return &Issue{
		Description:    fmt.Sprintf("%s is not excluded from git status (missing %q in %s)", wtDir, entry, excludePath),
		FixDescription: fmt.Sprintf("Add %q to %s", entry, excludePath),
		fixFn:          func() error { return excludeWorktreeDir(repo) },
	}
}

func checkDotBare(repo Repo) ([]Issue, error) {
	var issues []Issue
	outerDir := repo.LinkedWorktreeDir()

	// Check 2: outer .git file
	gitFile := filepath.Join(outerDir, ".git")
	wantContent := "gitdir: ./.bare\n"
	data, err := os.ReadFile(gitFile)
	switch {
	case os.IsNotExist(err):
		issues = append(issues, dotGitFileIssue(gitFile, wantContent,
			fmt.Sprintf("%s is missing", gitFile)))
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", gitFile, err)
	case string(data) != wantContent:
		issues = append(issues, dotGitFileIssue(gitFile, wantContent,
			fmt.Sprintf("%s has content %q, want %q", gitFile, string(data), wantContent)))
	}

	// Check 3: each worktree's .git file
	worktrees, err := ListWorktrees(repo)
	if err != nil {
		return nil, err
	}
	for _, wt := range worktrees {
		wt := wt
		wtIssues, err := checkWorktreeGitFile(repo, wt)
		if err != nil {
			return nil, err
		}
		issues = append(issues, wtIssues...)
	}

	return issues, nil
}

func dotGitFileIssue(gitFile, wantContent, description string) Issue {
	return Issue{
		Description:    description,
		FixDescription: fmt.Sprintf("Write %q to %s", wantContent, gitFile),
		fixFn:          func() error { return os.WriteFile(gitFile, []byte(wantContent), 0644) },
	}
}

func checkWorktreeGitFile(repo Repo, wt Worktree) ([]Issue, error) {
	wtGitFile := filepath.Join(wt.Path, ".git")
	// The canonical gitdir inside .bare for this worktree.
	wantGitDir := filepath.Clean(filepath.Join(repo.Root, "worktrees", wt.Name))
	// Relative path from the worktree dir to the gitdir (more portable).
	relGitDir, err := filepath.Rel(wt.Path, wantGitDir)
	if err != nil {
		relGitDir = wantGitDir // fall back to absolute
	}
	wantContent := fmt.Sprintf("gitdir: %s\n", relGitDir)

	fixIssue := func(description string) Issue {
		return Issue{
			Description:    description,
			FixDescription: fmt.Sprintf("Update %s to point to %s", wtGitFile, relGitDir),
			fixFn:          func() error { return os.WriteFile(wtGitFile, []byte(wantContent), 0644) },
		}
	}

	data, err := os.ReadFile(wtGitFile)
	if os.IsNotExist(err) {
		return []Issue{fixIssue(fmt.Sprintf("worktree %s is missing its .git file", wt.Name))}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", wtGitFile, err)
	}

	// Resolve the current gitdir path.
	line := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir: "), "\n")
	resolved := line
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(wt.Path, resolved)
	}
	resolved = filepath.Clean(resolved)

	if resolved != wantGitDir {
		return []Issue{fixIssue(fmt.Sprintf(
			"worktree %s .git file points to %q, want %q", wt.Name, resolved, wantGitDir,
		))}, nil
	}

	return nil, nil
}
