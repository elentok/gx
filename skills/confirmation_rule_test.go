package skills

import "testing"

func TestImplementNoConfirmationOfOwnRecommendationAnchors(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"Do not ask to confirm your own in-scope recommendation",
		"file it and finish",
		"Ask only for a real fork",
	)
}

func TestToTicketsSharedRegistryFileAnchors(t *testing.T) {
	checkAnchors(t, "gx-to-tickets/SKILL.md",
		"add entries to one registry file",
		"chain those tickets with `blocked_by`",
		"longer than about 3, add a prefactor ticket",
		"(prefactor-tickets.md)",
	)
	checkAnchors(t, "gx-to-tickets/prefactor-tickets.md",
		"more than about 3 tickets add entries to one registry file",
	)
}

func TestCodeReviewSpecDeviationsAreClaimsToCheckAnchors(t *testing.T) {
	checkAnchors(t, "gx-code-review/SKILL.md",
		"`gx tickets show <epic> --section \"Spec deviations\"`",
		"Pass the full list on, not a truncated view",
		"These are claims to check, not exclusions",
		"Verify each deviation against the code before skipping it",
		"Report a stale or wrong one as a finding",
	)
}

func TestImplementDiffCheckAnchors(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"## Check the diff against the ticket",
		"`Went wrong: Nothing` is allowed only when `## Spec deviations` is empty",
	)
}
