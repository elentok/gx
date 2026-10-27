package skills

import "testing"

func TestCwdRuleAnchors(t *testing.T) {
	for _, path := range []string{
		"gx-implement/SKILL.md",
		"gx-tdd/SKILL.md",
		"gx-resolving-merge-conflicts/SKILL.md",
	} {
		t.Run(path, func(t *testing.T) {
			checkAnchors(t, path,
				"Never leave the worktree root with a bare `cd`",
				"(cd webui && npx vitest ...)",
				"Read/Edit paths",
			)
		})
	}
}
