package skills

import "testing"

func TestResolvingMergeConflictsVerifiesKeptSide(t *testing.T) {
	checkAnchors(t, "gx-resolving-merge-conflicts/SKILL.md",
		"git show CHERRY_PICK_HEAD --stat",
		"--ours",
		"stated goal",
		"every package",
	)
}
