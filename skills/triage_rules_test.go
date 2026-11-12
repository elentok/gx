package skills

import (
	"strings"
	"testing"
)

func TestTriageSkillAnchors(t *testing.T) {
	skill := collapseSpace(readFile(t, "gx-triage-follow-ups/SKILL.md"))
	for _, want := range []string{
		"## 0. Link and detect the step", "## 1. Verify", "## 2. Plan", "## 3. Approve",
		"## 4. Write", "## 5. Done", "## Prefer the correct fix",
		"gx agent link --triage", "gx agent step", "gx agent model worker", "--fits",
		"`✅ <step>: <what was done>. Next: <what runs next>`",
		"never grills", `"auto": true`, "cheap to reverse", "(../gx-local-tracker.md)",
	} {
		if !strings.Contains(strings.ToLower(skill), strings.ToLower(want)) {
			t.Errorf("triage skill lacks %q", want)
		}
	}
}

func TestTriageWritePublishesThroughToTickets(t *testing.T) {
	skill := collapseSpace(readFile(t, "gx-triage-follow-ups/SKILL.md"))
	for _, want := range []string{
		"gx-to-tickets", "from:", "drop-rows", "retry at most once", "no retry", "Row loop",
	} {
		if !strings.Contains(strings.ToLower(skill), strings.ToLower(want)) {
			t.Errorf("triage skill lacks %q", want)
		}
	}
	for _, gone := range []string{"epic add <project>:follow-ups-<date>", "**Last pass**"} {
		if strings.Contains(skill, gone) {
			t.Errorf("triage skill still has %q", gone)
		}
	}
	toTickets := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	for _, want := range []string{"#### No spec mode", "skip `verify-plan`, `ensure-verify` and the quiz"} {
		if !strings.Contains(toTickets, normalizeSpace(want)) {
			t.Errorf("gx-to-tickets lacks %q", want)
		}
	}
}

func TestTrackerDocHasTriageRecord(t *testing.T) {
	doc := readFile(t, "gx-local-tracker.md")
	if !strings.Contains(doc, "\n## Triage record\n") {
		t.Error("gx-local-tracker.md lacks the `## Triage record` heading")
	}
	if !strings.Contains(doc, ".triage") || !strings.Contains(doc, "gx tickets triage") {
		t.Error("Triage record section lacks the .triage folder or the verbs")
	}
}
