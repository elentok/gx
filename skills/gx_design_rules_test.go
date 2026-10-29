package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestSkillsAskForTierNotModel(t *testing.T) {
	for _, path := range []string{
		"gx-design/SKILL.md", "gx-wayfinder/SKILL.md", "gx-code-review/SKILL.md",
		"gx-evolve/SKILL.md", "gx-triage-follow-ups/SKILL.md",
	} {
		if !strings.Contains(readFile(t, path), "gx agent model") {
			t.Errorf("%s lacks `gx agent model`", path)
		}
	}
	hardCoded := regexp.MustCompile(`model: ?"|"(sonnet|opus|haiku)"`)
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := hardCoded.Find(data); m != nil {
			t.Errorf("%s hard-codes a model: %q", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSkillConventionModelTier(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-local-tracker.md"))
	for _, want := range []string{
		"`gx agent model <tier>`",
		"`light`, `worker`, `strongest`",
		"leave out the `model` field",
		"the subagent runs on the session model",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-local-tracker.md lacks %q", want)
		}
	}
}

func TestGxDesignEndsRunAfterChartAndDestination(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-design/SKILL.md"))
	for _, want := range []string{
		"After charting, end the run with the run-end line",
		"end the run with the run-end line: never chain into step 3",
		"steps 1 and 2 end the run",
	} {
		if !strings.Contains(text, collapseSpace(want)) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
}

func TestGxDesignChartingSessionSubagentTiers(t *testing.T) {
	text := collapseSpace(readFile(t, "gx-design/SKILL.md"))
	start := strings.Index(text, "1. **The map has no `## Destination`**")
	end := strings.Index(text, "2. **`destination_unconfirmed: true`**")
	if start < 0 || end < start {
		t.Fatal("gx-design/SKILL.md lacks the chart step")
	}
	step := text[start:end]
	for _, want := range []string{
		"`gx agent model strongest`",
		"`gx agent model worker`",
		"gx-to-spec, gx-to-tickets and gx-research",
		"Research tickets spawned while charting also use `gx agent model worker`",
	} {
		if !strings.Contains(step, want) {
			t.Errorf("gx-design chart step lacks %q", want)
		}
	}
}

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

// gx-grilling names these two gx-design steps by number as the Destination grillings that always ask.
func TestGxDesignStepNumbersMatchGxGrilling(t *testing.T) {
	grilling := readFile(t, "gx-grilling/SKILL.md")
	for _, want := range []string{
		`"Chart the map" (gx-design step 1)`,
		"unconfirmed-Destination grilling (gx-design step 2)",
	} {
		if !strings.Contains(grilling, want) {
			t.Errorf("gx-grilling/SKILL.md lacks %q", want)
		}
	}
	design := readFile(t, "gx-design/SKILL.md")
	for _, want := range []string{
		"\n1. **The map has no `## Destination`** → report the step as `chart`.",
		"follow gx-wayfinder's \"Chart the map\"",
		"\n2. **`destination_unconfirmed: true`** → follow `gx-grilling` on the Destination alone",
	} {
		if !strings.Contains(design, want) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
}

func TestGxDesignSpecStepGrillsInPlace(t *testing.T) {
	text := readFile(t, "gx-design/SKILL.md")
	for _, want := range []string{
		"grill the person in this same session",
		"One round = one closed ticket",
		"`spec-round-`",
		"2-round cap",
		"A detour never resets it",
		"gx agent context-window --fits spec",
		"From the third draft on",
		"## Self-decided design questions",
		"disagree with, or one that changes user\n  stories or scope",
		"needs research or a prototype becomes an open",
		"then stop\n     without a redraft",
		"Set `--design-step spec` only after the draft is written",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
	for _, banned := range []string{"and stop, without chaining", "file them as `grilling` map tickets"} {
		if strings.Contains(text, banned) {
			t.Errorf("gx-design/SKILL.md still has %q", banned)
		}
	}
}

func TestGxDesignSecondReview(t *testing.T) {
	text := readFile(t, "gx-design/SKILL.md")
	for _, want := range []string{
		"go back\n   to \"Detecting the next step\"",
		"`(stories/scope changed)`",
		"`## Re-review findings`",
		"--design-step review\n   --allow-backward",
		"the first checklist\n   with its resolutions and the full spec",
		"only the changed stories and scope and what\n   they affect",
		"`- [x] No findings`",
		"Never run a third review",
		"neither findings section has an unchecked finding",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
	order := []string{"\n6. **", "\n6a. **", "\n6b. **", "\n7. **"}
	last := -1
	for _, marker := range order {
		i := strings.Index(text, marker)
		if i <= last {
			t.Errorf("gx-design/SKILL.md: %q missing or out of order", marker)
		}
		last = i
	}
	if strings.Contains(text, "continue to step 7 in the same run.\n7.") {
		t.Error("step 6 still continues straight to step 7")
	}
	if n := strings.Count(text, "--allow-backward` only for"); n != 1 {
		t.Errorf("--allow-backward rule appears %d times, want 1", n)
	}
	if !strings.Contains(text, "and for the move back to `review` for the second review") {
		t.Error("--allow-backward rule lacks the second case")
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

// grillingSkipRuleGaps returns the phrases of the skip-the-question rule that
// text lacks.
func grillingSkipRuleGaps(text string) []string {
	var gaps []string
	for _, want := range []string{
		"skip the question",
		"3 or more rounds",
		"is not counted",
		"non-recommended answer changed a user story, the scope or the map's Destination",
		"raised a new question, which then needed a later round",
		"a clarifying note, an objected low-risk default, and a second round that only confirms the first",
		"When you cannot tell, ask",
		`"Chart the map" (gx-design step 1)`,
		"unconfirmed-Destination grilling (gx-design step 2)",
		"A map ticket",
		"A `spec-round-` ticket",
		"`## Review findings` or `## Re-review findings`",
		"`(stories/scope changed)` marker",
		"print the summary once, as the last text before the next round or the run-end line",
		"no `❓ **Q` marker and no `gx-question` block",
		"`✅ ... Next:` line",
		"defines no reply to it",
		"`## Decisions so far`",
		"Do not reopen the grilling",
	} {
		if !strings.Contains(text, want) {
			gaps = append(gaps, want)
		}
	}
	return gaps
}

func TestGxGrillingSkipsSharedUnderstandingAfterStraightforwardGrilling(t *testing.T) {
	text := readFile(t, "gx-grilling/SKILL.md")
	for _, gap := range grillingSkipRuleGaps(text) {
		t.Errorf("gx-grilling/SKILL.md lacks %q", gap)
	}
	if strings.Contains(text, "Do not act on it until the user confirms") {
		t.Error("gx-grilling/SKILL.md still gates on the user confirming a shared understanding")
	}
}

func TestGxGrillingSkipRule_FailsWithoutDestinationException(t *testing.T) {
	text := readFile(t, "gx-grilling/SKILL.md")
	broken := strings.ReplaceAll(text, "unconfirmed-Destination grilling (gx-design step 2)", "")
	if len(grillingSkipRuleGaps(broken)) == 0 {
		t.Error("skip rule check passes without the Destination exception")
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
