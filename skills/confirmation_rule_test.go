package skills

import "testing"

func TestImplementNoConfirmationOfOwnRecommendationAnchors(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"Do not ask to confirm your own in-scope recommendation",
		"file it and finish",
		"Ask only for a real fork",
	)
}

func TestImplementDiffCheckAnchors(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"## Check the diff against the ticket",
		"`Went wrong: Nothing` is allowed only when `## Spec deviations` is empty",
	)
}
