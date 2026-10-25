package skills

import (
	"strings"
	"testing"
)

func TestGxDesignChainingRules(t *testing.T) {
	text := readFile(t, "gx-design/SKILL.md")
	for _, want := range []string{
		"gx agent context-window --fits",
		"exit 0",
		"gx agent step",
		"before printing the run-end line",
		"`--allow-backward` only for a review finding that becomes a map ticket",
		"Skip claimed tickets",
		"never enters a prototype or HITL task ticket",
		"A run that starts on one ends after it",
		"picks only the first ticket",
		"last map ticket",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
	for _, banned := range []string{"50K", "Never chain out of steps 1", "exactly **one** ticket"} {
		if strings.Contains(text, banned) {
			t.Errorf("gx-design/SKILL.md still has %q", banned)
		}
	}
}

func TestGxGrillingLowRiskDefaults(t *testing.T) {
	text := readFile(t, "gx-grilling/SKILL.md")
	for _, want := range []string{
		"decided unless you object",
		"changes no user story, no scope and not the Destination",
		"cheap to reverse",
		"When in doubt, ask",
		"**Decided unless you object**",
		"Accept these defaults?",
		"An objected default is asked like any other question",
		"Decided by default",
		"(default)",
	} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(want)) {
			t.Errorf("gx-grilling/SKILL.md lacks %q", want)
		}
	}
	if !strings.Contains(readFile(t, "gx-design/SKILL.md"), `"(default)"`) {
		t.Error("gx-design/SKILL.md step 6 lacks the (default) rule")
	}
}

func TestGxWayfinderLetsCallerAllowMoreTickets(t *testing.T) {
	text := readFile(t, "gx-wayfinder/SKILL.md")
	if !strings.Contains(text, "unless the calling skill allows more") {
		t.Error("gx-wayfinder/SKILL.md lacks the calling-skill exception")
	}
	if strings.Contains(text, "never resolve more than one ticket per session**, with the exception") {
		t.Error("gx-wayfinder/SKILL.md still has the old one-ticket rule")
	}
}
