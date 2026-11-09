package skills

import "testing"

func TestImplementNoConfirmationOfOwnRecommendationAnchors(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"Do not ask to confirm your own in-scope recommendation",
		"file it and finish",
		"Ask only for a real fork",
	)
}
