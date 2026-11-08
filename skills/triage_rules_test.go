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
		"never grills", "(../gx-local-tracker.md)",
	} {
		if !strings.Contains(strings.ToLower(skill), strings.ToLower(want)) {
			t.Errorf("triage skill lacks %q", want)
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
