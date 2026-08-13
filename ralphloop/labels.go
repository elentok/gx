package ralphloop

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
)

// iterLabel/iterBranch/conflictLabel key off a ticket's Identifier (the
// filename's full "NN[letter]" prefix), not its Number, so that lettered
// split siblings sharing the same Number (e.g. "04a"/"04b") get distinct
// labels/branches/worktree paths instead of colliding on "iter-04".
//
// iterLabel is scoped by epicName, using the same flat "{epic}-..." form as
// iterItemName, because this label is what's actually shown to the user
// (herdr tab/pane names): two epics that each happen to reach iteration "06"
// would otherwise be indistinguishable wherever the label surfaces.
//
// herdr's agent name cap is 32 chars; a long epicName plus "-iter-" plus the
// identifier can exceed that ("invalid_agent_name", every ticket in the epic
// then fails to launch). When the full label doesn't fit, epicName is
// truncated and given a short content hash suffix — deterministic (every
// call site recomputes the same label independently, e.g. reconcile.go
// matching live herdr tabs) and collision-resistant (two epic names that
// happen to share a long prefix truncate to different labels).
const maxIterLabelLen = 32

func iterLabel(epicName, identifier string) string {
	suffix := "-iter-" + identifier
	budget := maxIterLabelLen - len(suffix)
	name := epicName
	if len(name) > budget {
		hashSuffix := fmt.Sprintf("-%x", sha1.Sum([]byte(epicName)))[:7]
		name = epicName[:budget-len(hashSuffix)] + hashSuffix
	}
	return name + suffix
}

// iterItemName scopes an iteration by epicName as a single flat path/branch
// segment ("{epic}-item-{id}") rather than "{epic}/iter-{id}", so an
// iteration's worktree never lands as a subdirectory of the epic's own
// feature worktree (which lives at "{epic}") and its branch never nests
// under the epic's own branch (named plain "{epic}").
func iterItemName(epicName, identifier string) string {
	return epicName + "-item-" + identifier
}

// iterationWorktreePath returns an iteration's on-disk worktree path, scoped
// by epicName. worktreeDir is shared across every epic running against the
// same repo (unlike the herdr workspace, which is already one-per-epic), so
// without this two epics that both happen to use the same iteration number
// (e.g. two "iter-04"s) would collide on the same directory. It's a sibling
// of worktreeDir/epicName (the feature worktree), not nested inside it: git
// operations (status/clean/add) run in the feature worktree must never see
// live iteration worktrees as foreign content within its own tree.
func iterationWorktreePath(worktreeDir, epicName, identifier string) string {
	return filepath.Join(worktreeDir, iterItemName(epicName, identifier))
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
	return "ralph-loop/" + iterItemName(epicName, identifier)
}

func conflictLabel(identifier string) string {
	return "conflict-" + identifier
}
