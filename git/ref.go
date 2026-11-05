package git

import "github.com/elentok/gx/git/internal/run"

// ValidBranchName reports whether git accepts name as a branch name.
func ValidBranchName(name string) bool {
	_, _, err := run.Exec("", []string{"check-ref-format", "--branch", name})
	return err == nil
}

// RefExists reports whether the full ref (e.g. refs/heads/main) exists.
// Unlike IsLocalBranch it never matches a pattern.
func RefExists(repoRoot, ref string) bool {
	_, _, err := run.Exec(repoRoot, []string{"rev-parse", "--verify", "--quiet", ref})
	return err == nil
}
