package skills

import "testing"

func TestResolvingMergeConflictsVerifiesKeptSide(t *testing.T) {
	checkAnchors(t, "gx-resolving-merge-conflicts/SKILL.md",
		"git show CHERRY_PICK_HEAD --stat",
		"--ours",
		"stated goal",
		"every package",
		"`gx-verify` skill on every conflicted file",
		"*.e2e.ts",
		"before `git cherry-pick --continue`",
	)
}
