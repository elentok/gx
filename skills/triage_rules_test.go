package skills

import (
	"strings"
	"testing"
)

func TestTriageSkillAnchors(t *testing.T) {
	skill := collapseSpace(readFile(t, "gx-triage-follow-ups/SKILL.md"))
	for _, want := range []string{
		"## 0. Link and detect the step", "## 1. Verify", "## 2. Route", "## 3. Questions",
		"## 4. Write", "## 5. Done", "## Prefer the correct fix",
		"gx agent link --triage", "gx agent step <verify|route|questions|write|done>",
		"gx agent model worker", "--fits", "Decided unless you object", "Write this plan?",
		"`✅ <step>: <what was done>. Next: <what runs next>`",
		"never grills outside the Questions step", "expensive to reverse",
		"(old-epic-areas.txt)", "gx tickets triage oldepics", "decided by agent",
		"(../gx-local-tracker.md)",
	} {
		if n := strings.Count(strings.ToLower(skill), strings.ToLower(want)); n < 1 {
			t.Errorf("triage skill lacks %q", want)
		}
	}
	for _, gone := range []string{
		"## 2. Plan", "## 3. Approve", `"auto": true`, "`quick`", "send-back", "`earlier`",
		"more than 2 questions", "--kind map",
	} {
		if strings.Contains(skill, gone) {
			t.Errorf("triage skill still has %q", gone)
		}
	}
}

func TestTriageSkillTeachesGrouping(t *testing.T) {
	skill := collapseSpace(readFile(t, "gx-triage-follow-ups/SKILL.md"))
	for _, want := range []string{"`estimate`", "--ungroup", "oldest first"} {
		if n := strings.Count(skill, want); n != 1 {
			t.Errorf("triage skill has %q %d times, want once", want, n)
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
