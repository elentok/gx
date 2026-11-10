package skills

import "testing"

func TestPRFlowSkillRules(t *testing.T) {
	checkAnchors(t, "gx-ci-fix/SKILL.md",
		"never skip hooks",
		"never overwrite foreign commits",
		"never edit CI config to turn a check off",
		"Run the failing check locally",
		"`gx agent model strongest`",
		"read-only reviewer",
		"`--force-with-lease`",
		"`Check`", "`Cause`", "`Change`", "`Commit`", "`Outcome`",
		"`fixed`", "`unfixable`", "`foreign-conflict`",
	)
	checkAnchors(t, "gx-cleanup/SKILL.md",
		"`gx server epics mark-merged <project:epic>`",
		"never writes `merged_at` itself",
	)
	checkAnchors(t, "gx-pr-text/SKILL.md", "`Title:`", "`Body:`", "`skills.pr-text`")
}
