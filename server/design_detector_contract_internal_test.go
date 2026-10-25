package server

import (
	"regexp"
	"strings"
	"testing"

	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/testutil/agentfake"
)

// The skills print the text the design detector reads. These tests read the
// skills from the embedded bundle, so a wording change that breaks detection
// fails here.

// runEndShape is the structure the gx-design skill gives its closing line.
var runEndShape = regexp.MustCompile(`^✅ [^:\n]+: .+ Next: .+$`)

func skillText(t *testing.T, name string) string {
	t.Helper()
	b, err := skills.Bundle.ReadFile(name + "/SKILL.md")
	if err != nil {
		t.Fatalf("read skill %s: %v", name, err)
	}
	return string(b)
}

// fencedBlocks returns the contents of every block opened and closed by a line
// that is exactly fence.
func fencedBlocks(md, fence string) []string {
	var blocks []string
	var cur []string
	open := false
	for _, line := range strings.Split(md, "\n") {
		switch {
		case line == fence && open:
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur, open = nil, false
		case line == fence:
			open = true
		case open:
			cur = append(cur, line)
		}
	}
	return blocks
}

// afterEveryRun returns the template block and the examples block of the
// gx-design "After every run" section.
func afterEveryRun(t *testing.T) (template string, examples []string) {
	t.Helper()
	_, section, ok := strings.Cut(skillText(t, "gx-design"), "## After every run")
	if !ok {
		t.Fatal("gx-design has no \"After every run\" section")
	}
	blocks := fencedBlocks(section, "```")
	if len(blocks) < 2 {
		t.Fatalf("After every run has %d fenced blocks, want the template and the examples", len(blocks))
	}
	return blocks[0], strings.Split(strings.TrimSpace(blocks[1]), "\n")
}

func continues(text string) bool {
	a, ok := designActionable(text, "gx:epic/03", false)
	return ok && !a.Grilling && len(a.Action) == 2
}

func TestGxDesignRunEndTemplate_IsContinue(t *testing.T) {
	template, _ := afterEveryRun(t)
	if !runEndShape.MatchString(template) {
		t.Fatalf("template %q does not have the ✅ <label>: ... Next: ... shape", template)
	}
	if !continues(template) {
		t.Errorf("template %q does not yield Continue", template)
	}
}

func TestGxDesignRunEndExamples_AreContinue(t *testing.T) {
	_, examples := afterEveryRun(t)
	if len(examples) < 2 {
		t.Fatalf("got %d examples, want at least 2", len(examples))
	}
	for _, ex := range examples {
		if !continues(ex) {
			t.Errorf("example %q does not yield Continue", ex)
		}
	}
}

func TestGxDesignRunEndTemplate_WithoutNextFails(t *testing.T) {
	template, _ := afterEveryRun(t)
	mutated := strings.ReplaceAll(template, "Next:", "")
	if continues(mutated) {
		t.Errorf("mutated template %q still yields Continue", mutated)
	}
	if runEndShape.MatchString(mutated) {
		t.Errorf("mutated template %q still matches the template shape", mutated)
	}
}

func TestAgentfakeDesignRunEnd_HasTemplateShape(t *testing.T) {
	if !runEndShape.MatchString(agentfake.DesignRunEnd) {
		t.Errorf("agentfake.DesignRunEnd %q does not have the template shape", agentfake.DesignRunEnd)
	}
	if !runEndLine.MatchString(agentfake.DesignRunEnd) {
		t.Errorf("agentfake.DesignRunEnd %q does not match runEndLine", agentfake.DesignRunEnd)
	}
}

func TestGxGrillingRoundTemplate_IsGrilling(t *testing.T) {
	blocks := fencedBlocks(skillText(t, "gx-grilling"), "````")
	if len(blocks) == 0 {
		t.Fatal("gx-grilling has no question-round template")
	}
	a, ok := designActionable(blocks[0], "gx:epic/03", false)
	if !ok || !a.Grilling || a.Reason != grillingReason {
		t.Errorf("round template yields (%+v, %v), want the grilling Actionable", a, ok)
	}
}

func TestAgentfakeDefaultedRounds_UseTheRealFormat(t *testing.T) {
	for turn, wantCards := range map[string]int{
		agentfake.DesignTurnDefaulted: 1,
		agentfake.DesignTurnMixed:     1,
	} {
		text := agentfake.DesignRoundText(turn)
		before, after, ok := strings.Cut(text, "\n---\n")
		if !ok || !isDefaultsHeading(strings.TrimSpace(after)) {
			t.Errorf("%s round has no defaults heading after a --- line: %q", turn, text)
			continue
		}
		if !strings.Contains(before, "❓") {
			t.Errorf("%s round = %q, want the card before the defaults", turn, text)
		}
		cards := roundCards(text)
		if len(cards) != wantCards || strings.Contains(cards[0].Body, "Q10") {
			t.Errorf("%s round cards = %+v, want %d card without the defaults", turn, cards, wantCards)
		}
	}
}

func TestGxGrillingAllDefaultedTemplate_IsGrilling(t *testing.T) {
	var found string
	for _, b := range fencedBlocks(skillText(t, "gx-grilling"), "````") {
		if strings.Contains(b, "Accept these defaults?") {
			found = b
		}
	}
	if found == "" {
		t.Fatal("gx-grilling has no all-defaulted round template")
	}
	a, ok := designActionable(found, "gx:epic/03", false)
	if !ok || !a.Grilling || a.Reason != grillingReason {
		t.Errorf("all-defaulted template yields (%+v, %v), want the grilling Actionable", a, ok)
	}
	if cards := roundCards(found); len(cards) != 1 || strings.Contains(cards[0].Body, "Decided unless") {
		t.Errorf("all-defaulted template cards = %+v, want one card without the defaults", cards)
	}
}
