package ralphloop

// iterLabel/iterBranch/conflictLabel key off a ticket's Identifier (the
// filename's full "NN[letter]" prefix), not its Number, so that lettered
// split siblings sharing the same Number (e.g. "04a"/"04b") get distinct
// labels/branches/worktree paths instead of colliding on "iter-04".
func iterLabel(identifier string) string {
	return "iter-" + identifier
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
