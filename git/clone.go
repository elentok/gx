package git

import (
	"os"
	"path/filepath"
	"strings"
)

// CloneBare clones repoURL using the .bare trick:
//
//	my-repo/
//	  .bare/   ← actual bare git repo
//	  .git      ← text file: "gitdir: ./.bare"
//	  main/    ← linked worktrees live here
//
// targetDir, if non-empty, sets the outer directory name. Otherwise the name
// is inferred from the URL (with any ".git" suffix stripped).
// Returns the path to the outer directory (e.g. my-repo/).
func CloneBare(repoURL, targetDir, cwd string) (string, error) {
	outerName := targetDir
	if outerName == "" {
		outerName = inferCloneDirFromURL(repoURL)
		outerName = strings.TrimSuffix(outerName, ".git")
	}

	outerDir := outerName
	if !filepath.IsAbs(outerDir) {
		outerDir = filepath.Join(cwd, outerDir)
	}

	if err := os.MkdirAll(outerDir, 0755); err != nil {
		return "", err
	}

	bareDir := filepath.Join(outerDir, ".bare")
	if _, _, err := run(cwd, []string{"clone", "--bare", repoURL, bareDir}); err != nil {
		return "", err
	}

	// Write the .git file so git recognises the outer directory as a repo root.
	gitFile := filepath.Join(outerDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ./.bare\n"), 0644); err != nil {
		return "", err
	}

	// git clone --bare sets the fetch refspec to "+refs/heads/*:refs/heads/*",
	// which fetches remote branches directly into local refs rather than into
	// refs/remotes/origin/*. This means remote tracking refs never get
	// populated, so ahead/behind status and upstream tracking won't work.
	// We fix the refspec immediately after cloning so that subsequent fetches
	// behave like a normal clone.
	if _, _, err := run(bareDir, []string{"config", "remote.origin.fetch", expectedFetchRefspec}); err != nil {
		return "", err
	}

	// Populate refs/remotes/origin/* now that the refspec is correct.
	if _, _, err := run(bareDir, []string{"fetch", "origin"}); err != nil {
		return "", err
	}

	// Set upstream tracking for all local branches so that `git pull` works
	// without having to specify a remote. git clone --bare doesn't configure
	// branch.<name>.remote / branch.<name>.merge, so we do it manually.
	if stdout, _, err := run(bareDir, []string{"branch", "--format=%(refname:short)"}); err == nil {
		for _, branch := range strings.Fields(stdout) {
			// Ignore errors: if origin/<branch> doesn't exist, skip it.
			_, _, _ = run(bareDir, []string{"branch", "--set-upstream-to=origin/" + branch, branch})
		}
	}

	return outerDir, nil
}

func inferCloneDirFromURL(repoURL string) string {
	s := strings.TrimSuffix(repoURL, "/")
	i := strings.LastIndexAny(s, "/:")
	name := s
	if i >= 0 && i+1 < len(s) {
		name = s[i+1:]
	}
	return name
}

// AddWorktreeFromRemote adds a worktree for branch. It first tries to check out
// an existing local branch, then falls back to creating a tracking branch from
// remoteBranch (for freshly cloned bare repositories).
func AddWorktreeFromRemote(repo Repo, worktreePath, branch, remoteBranch string) error {
	if err := excludeWorktreeDir(repo); err != nil {
		return err
	}
	if _, _, err := run(repo.Root, []string{"worktree", "add", worktreePath, branch}); err == nil {
		return nil
	}
	_, _, err := run(repo.Root, []string{"worktree", "add", "--track", "-b", branch, worktreePath, remoteBranch})
	return err
}
