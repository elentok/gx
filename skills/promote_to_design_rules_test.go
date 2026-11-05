package skills

import (
	"regexp"
	"strings"
	"testing"
)

var promotedSentenceRe = regexp.MustCompile("(?m)^`(Promoted[^`]+)`$")

// promotedSentenceGaps reports how the skill's fixed first-line sentence and
// the wayfinder rule that quotes it disagree.
func promotedSentenceGaps(skill, wayfinder string) []string {
	m := promotedSentenceRe.FindStringSubmatch(skill)
	if m == nil {
		return []string{"the skill's fixed first-line sentence"}
	}
	if !strings.Contains(collapseSpace(wayfinder), "`"+collapseSpace(m[1])+"`") {
		return []string{"the wayfinder rule quoting " + m[1]}
	}
	return nil
}

func TestPromotedSentenceSharedByWayfinder(t *testing.T) {
	for _, gap := range promotedSentenceGaps(readFile(t, "gx-promote-to-design/SKILL.md"), readFile(t, "gx-wayfinder/SKILL.md")) {
		t.Errorf("drift: lacks %s", gap)
	}
}

func TestPromotedSentenceDriftIsCaught(t *testing.T) {
	skill := readFile(t, "gx-promote-to-design/SKILL.md")
	wayfinder := readFile(t, "gx-wayfinder/SKILL.md")
	m := promotedSentenceRe.FindStringSubmatch(skill)
	if m == nil {
		t.Fatal("skill lacks the fixed sentence")
	}
	drifted := strings.ReplaceAll(wayfinder, m[1], m[1]+" Changed.")
	if len(promotedSentenceGaps(skill, drifted)) == 0 {
		t.Error("check passes when the wayfinder sentence differs")
	}
}

func TestGxPromoteToDesignSkill(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-promote-to-design/SKILL.md"))
	for _, want := range []string{
		"gx agent actionable",
		"gx designs open <slug> --idea - --project <project>",
		"gx server web-url --json",
		"gx agent transcript --json",
		"## Failure handling",
		"## The brief",
		"[gx-grilling](../gx-grilling/SKILL.md)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-promote-to-design/SKILL.md lacks %q", want)
		}
	}
	if !strings.Contains(collapseSpace(readFile(t, "gx-wayfinder/SKILL.md")), "[gx-promote-to-design](../gx-promote-to-design/SKILL.md)") {
		t.Error("gx-wayfinder/SKILL.md does not link the promote skill")
	}
}
