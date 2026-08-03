package ralphloop

import "path/filepath"

// iterLabel/iterBranch/conflictLabel key off a ticket's Identifier (the
// filename's full "NN[letter]" prefix), not its Number, so that lettered
// split siblings sharing the same Number (e.g. "04a"/"04b") get distinct
// labels/branches/worktree paths instead of colliding on "iter-04".
func iterLabel(identifier string) string {
	return "iter-" + identifier
}

// iterationWorktreePath returns an iteration's on-disk worktree path, scoped
// by epicName. worktreeDir is shared across every epic running against the
// same repo (unlike the herdr workspace, which is already one-per-epic), so
// without this two epics that both happen to use the same iteration number
// (e.g. two "iter-04"s) would collide on the same directory.
func iterationWorktreePath(worktreeDir, epicName, identifier string) string {
	return filepath.Join(worktreeDir, epicName, iterLabel(identifier))
}

// iterationKey scopes an iteration label by epic name for use as a
// live-tab-tracking map key, so reconcile can't mistake one epic's live tab
// for another epic's same-numbered iteration.
func iterationKey(epicName, label string) string {
	return epicName + "/" + label
}

// iterBranch is scoped by epicName (unlike iterLabel, which is only ever
// compared within one epic's own herdr workspace) because git branches live
// in the one shared bare repo across every worktree: two epics that happen
// to reuse the same ticket identifier (e.g. an auto-split "06b") would
// otherwise collide on the same branch name and fail AddWorktree.
func iterBranch(epicName, identifier string) string {
	return "ralph-loop/" + epicName + "/" + iterLabel(identifier)
}

func conflictLabel(identifier string) string {
	return "conflict-" + identifier
}
