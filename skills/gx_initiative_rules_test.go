package skills

import (
	"strings"
	"testing"
)

func initiativeSkill(t *testing.T) string {
	t.Helper()
	return collapseSpace(readFile(t, "gx-initiative/SKILL.md"))
}

func requireAll(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("gx-initiative/SKILL.md lacks %q", want)
		}
	}
}

// section returns the text from start up to (not including) the next occurrence of end after it.
func section(t *testing.T, text, start, end string) string {
	t.Helper()
	i := strings.Index(text, start)
	if i < 0 {
		t.Fatalf("gx-initiative/SKILL.md lacks %q", start)
	}
	rest := text[i:]
	if end == "" {
		return rest
	}
	j := strings.Index(rest[len(start):], end)
	if j < 0 {
		t.Fatalf("gx-initiative/SKILL.md lacks %q after %q", end, start)
	}
	return rest[:len(start)+j]
}

func TestGxInitiativeManualPolicy(t *testing.T) {
	if !strings.Contains(readFile(t, "gx-initiative/SKILL.md"), "disable-model-invocation: true") {
		t.Error("frontmatter lacks disable-model-invocation: true")
	}
	if !strings.Contains(readFile(t, "gx-initiative/agents/openai.yaml"), "allow_implicit_invocation: false") {
		t.Error("openai.yaml lacks allow_implicit_invocation: false")
	}
	if !strings.Contains(collapseSpace(readFile(t, "gx.md")), "`gx-initiative` sets it as well") {
		t.Error("gx.md invocation policy does not name gx-initiative")
	}
}

func TestGxInitiativeDetectionOrder(t *testing.T) {
	text := initiativeSkill(t)
	markers := []string{
		"1. **Broken link or unparseable Milestones section.**",
		"2. **No Milestone has a Design: shape.**",
		"3. **Gate: the latest Milestone with a Design is merged and has no gate note.**",
		"4. **Resume: the latest gated Milestone",
		"5. **Otherwise**",
	}
	last := -1
	for _, m := range markers {
		if strings.Count(text, m) != 1 {
			t.Errorf("%q appears %d times, want 1", m, strings.Count(text, m))
		}
		i := strings.Index(text, m)
		if i < last {
			t.Errorf("%q is out of order", m)
		}
		last = i
	}
	requireAll(t, text, "The first one that matches runs and the rest are skipped",
		"Do not repeat the Gate", "print a pointer to the next step and stop")
}

func TestGxInitiativeBrokenLinkStep(t *testing.T) {
	step := section(t, initiativeSkill(t), "1. **Broken link", "2. **No Milestone")
	requireAll(t, step,
		"Check the full Milestone list, not one link",
		"A Milestone whose Design epic is not in the store",
		"gx initiatives milestone set <project:initiative> <slug> --clear-design",
		"write nothing")
}

func TestGxInitiativeSessionAndGrilling(t *testing.T) {
	text := initiativeSkill(t)
	requireAll(t, text,
		"[gx-grilling](../gx-grilling/SKILL.md)",
		"[gx-domain-modeling](../gx-domain-modeling/SKILL.md)",
		"[gx-unslop](../gx-unslop/SKILL.md)",
		"in the `gx-grilling` format",
		"with a recommended answer",
		"Ask the person nothing in prose outside a round",
		"no worktree, no `gx agent link`, no Flow, no Progress, no Continue or auto-advance")
}

func TestGxInitiativeShapeStep(t *testing.T) {
	step := section(t, initiativeSkill(t), "2. **No Milestone", "3. **Gate")
	requireAll(t, step,
		"the Destination, the Milestone list (a name and a one-line goal each), the Assumptions, and which Milestone is first",
		"Check only that Milestone 1 fits one reviewable PR",
		"Later Milestones stay sketches",
		"A spike is an ordinary first Milestone the person names",
		"no special kind",
		"[Create the Design](#create-the-design)",
		"no Design exists before that confirmation")
}

func TestGxInitiativeCreateDesignSection(t *testing.T) {
	text := initiativeSkill(t)
	if n := strings.Count(text, "## Create the Design"); n != 1 {
		t.Errorf("Create the Design section appears %d times, want 1", n)
	}
	if n := strings.Count(text, "[Create the Design](#create-the-design)"); n != 2 {
		t.Errorf("Create the Design is referenced %d times, want 2 (shape and resume)", n)
	}
	resume := section(t, text, "4. **Resume", "5. **Otherwise**")
	requireAll(t, resume, "[Create the Design](#create-the-design)")
	create := section(t, text, "## Create the Design", "")
	requireAll(t, create,
		"`<initiative>-<milestone-slug>`", "`-design` name", "`-impl` name",
		"before any write",
		"`initiative:` and `milestone:`",
		"`## Initial prompt`",
		"the Milestone goal", "the Assumptions it tests",
		"open deferred items whose `to:` is that Milestone, shown as candidates and not decisions",
		"orphan", "do not refuse",
		"gx initiatives milestone set <project:initiative> <slug> --design",
		"Never create a Design for any other Milestone")
}

func TestGxInitiativeGateStep(t *testing.T) {
	step := section(t, initiativeSkill(t), "3. **Gate", "4. **Resume")
	requireAll(t, step,
		"merged and has no gate note",
		"from the spec, the impl epic's tickets, the merged PR and the verify results",
		"as evidence. Never copy them",
		"each Assumption this Milestone tests and any other Assumption the evidence touches",
		"whose `to:` is the gated Milestone or the next Milestone, and every `unplaced` one",
		"Write nothing before the answers",
		"`continue`, `reshape`, `stop`, or `done`",
		"before the placement round",
		"sets every open deferred item to `dropped` with that reason",
		"\"Initiative done\"",
		"One gate per Milestone, never re-run")
	reshape := strings.Index(step, "3. If the outcome is `reshape`")
	placement := strings.Index(step, "4. Placement round")
	write := strings.Index(step, "Write nothing before the answers")
	if reshape < 0 || reshape > placement || placement > write {
		t.Error("Gate order must be reshape edits, then the placement round, then the write step")
	}
}

func TestGxInitiativeUsesVerbsOnly(t *testing.T) {
	text := initiativeSkill(t)
	requireAll(t, text,
		"Edit the Initiative only through the `gx initiatives` verbs",
		"Never format a Milestone, Assumption or deferred line yourself",
		"Never edit the Initiative with `gx tickets section`")
	for _, banned := range []string{"three-value", "scope rule"} {
		if strings.Contains(text, banned) {
			t.Errorf("SKILL.md carries %q, which belongs to gx-design", banned)
		}
	}
}

func TestGxInitiativeNeverShapesTwice(t *testing.T) {
	requireAll(t, initiativeSkill(t),
		"Never create a Design for any other Milestone",
		"the pointer names that Design, and the skill shapes nothing twice")
}
