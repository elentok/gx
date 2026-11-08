package cleanupscan

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
)

// Branch deletion modes for `git branch`. DeleteNone means the branch is not
// proven merged and must not be deleted from here.
const (
	DeleteSafe  = "-d"
	DeleteForce = "-D"
	DeleteNone  = ""
)

// MergeCheck is the outcome of CheckMerged.
type MergeCheck struct {
	Merged bool `json:"merged"`
	// DeleteMode is DeleteSafe only when ancestry shows the branch merged into
	// trunk; every other proven merge needs DeleteForce, because git's own
	// check only looks at ancestry.
	DeleteMode string `json:"delete_mode,omitempty"`
}

// CheckMerged reports whether branch is merged and which `git branch` flag
// deletes it. Merged means ancestry, or one of two patch-id checks against a
// baseline: per-commit (a cherry-pick) or whole-branch (a squash merge). The
// baseline is epicName's branch when it exists (pass "" for none), then trunk.
// Safe to call again from a write route to re-prove a merge before deleting.
func CheckMerged(repo git.Repo, branch, epicName string) (MergeCheck, error) {
	anc, err := git.IsAncestor(repo.Root, branch, repo.Trunk)
	if err != nil {
		return MergeCheck{}, err
	}
	if anc {
		return MergeCheck{Merged: true, DeleteMode: DeleteSafe}, nil
	}

	baselines := []string{}
	if epicName != "" && git.IsLocalBranch(repo.Root, epicName) {
		baselines = append(baselines, epicName)
	}
	baselines = append(baselines, repo.Trunk)

	for _, baseline := range baselines {
		merged, err := mergedInto(repo.Root, branch, baseline)
		if err != nil {
			return MergeCheck{}, err
		}
		if merged {
			return MergeCheck{Merged: true, DeleteMode: DeleteForce}, nil
		}
	}
	return MergeCheck{}, nil
}

func mergedInto(dir, branch, baseline string) (bool, error) {
	anc, err := git.IsAncestor(dir, branch, baseline)
	if err != nil || anc {
		return anc, err
	}
	base, err := git.MergeBase(dir, branch, baseline)
	if err != nil {
		// No common history: nothing to compare against.
		return false, nil
	}
	if applied, err := history.PatchesApplied(dir, baseline, base, branch); err != nil || applied {
		return applied, err
	}
	return history.SquashApplied(dir, baseline, base, branch)
}
