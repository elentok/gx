package skills

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/ticketspec"
)

func TestToTicketsPrefactorChecks(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/prefactor-tickets.md"))
	for _, anchor := range []string{"#### Prefactor tickets", "`sizeguard`", "gopls", "the export decision"} {
		if !strings.Contains(text, normalizeSpace(anchor)) {
			t.Errorf("gx-to-tickets/prefactor-tickets.md is missing %q", anchor)
		}
	}
}

func TestCodeReviewFixTicketRules(t *testing.T) {
	toTickets := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	for _, anchor := range []string{"### 4d. Code-review fix tickets", "--blocked-by <earlier-id>", "never batch numbers"} {
		if !strings.Contains(toTickets, normalizeSpace(anchor)) {
			t.Errorf("gx-to-tickets/SKILL.md is missing %q", anchor)
		}
	}
	review := normalizeSpace(readFile(t, "gx-code-review/SKILL.md"))
	if !strings.Contains(review, "(../gx-to-tickets/SKILL.md)") {
		t.Error("gx-code-review/SKILL.md must link the fix-ticket rules in gx-to-tickets")
	}
}

// Each rare-guidance file is linked once from SKILL.md and the link resolves.
func TestToTicketsLinkedFiles(t *testing.T) {
	skill := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	for _, name := range []string{"prefactor-tickets.md", "wide-refactors.md", "quiz-format.md", "verify-planning.md"} {
		if got := strings.Count(skill, "]("+name+")"); got != 1 {
			t.Errorf("gx-to-tickets/SKILL.md links %q %d times, want 1", name, got)
		}
		readFile(t, "gx-to-tickets/"+name) // fails the test if missing
	}
	wide := normalizeSpace(readFile(t, "gx-to-tickets/wide-refactors.md"))
	if !strings.Contains(wide, "## Wide refactors: expand–contract") {
		t.Error("wide-refactors.md is missing its heading")
	}
}

func TestToTicketsQuizSkip(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	for _, anchor := range []string{"### 5. Quiz the user (only when a trigger fires)", "`quiz skipped: no trigger fired`"} {
		if got := strings.Count(text, normalizeSpace(anchor)); got != 1 {
			t.Errorf("gx-to-tickets/SKILL.md has %d copies of %q, want 1", got, anchor)
		}
	}
	// The trigger list is stated once: one "over 45K tokens" trigger.
	if got := strings.Count(text, "over 45K tokens"); got != 1 {
		t.Errorf("trigger list appears %d times, want 1", got)
	}
}

func TestGxDesignStep7PassesNoQuizRule(t *testing.T) {
	text := readFile(t, "gx-design/SKILL.md")
	i := strings.Index(text, "\n7. **No `<epic>-impl` epic")
	j := strings.Index(text, "\n8. **No `<epic>-impl` branch**")
	if i < 0 || j < i {
		t.Fatal("gx-design/SKILL.md step 7 not found")
	}
	step := strings.ToLower(text[i:j])
	for _, bad := range []string{"quiz", "trigger"} {
		if strings.Contains(step, bad) {
			t.Errorf("gx-design step 7 mentions %q", bad)
		}
	}
}

func TestToTicketsDraftWriters(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	i := strings.Index(text, "### 4c.")
	j := strings.Index(text, "### 4d.")
	k := strings.Index(text, "### 5.")
	if i < 0 || j < i || k < j {
		t.Fatal("gx-to-tickets/SKILL.md steps 4c, 4d, 5 not found in order")
	}
	if !strings.Contains(text[i:j], "gx tickets draft") {
		t.Error("step 4c does not point at `gx tickets draft`")
	}
	if strings.Contains(text[i:j], "with `rg` or `fd`") {
		t.Error("step 4c still lists a per-claim rg check")
	}
	for _, anchor := range []string{"4 or more", "min(3, ticket count)", "never edits slugs or `blocked_by`", "gx agent model worker"} {
		if got := strings.Count(text, normalizeSpace(anchor)); got != 1 {
			t.Errorf("gx-to-tickets/SKILL.md has %d copies of %q, want 1", got, anchor)
		}
	}
	const rule = "no separate explore or draft subagent"
	step2 := text[strings.Index(text, "### 2."):strings.Index(text, "### 3.")]
	if !strings.Contains(step2, rule) {
		t.Error("step 2 lacks the no separate explore or draft subagent rule")
	}
	design := normalizeSpace(readFile(t, "gx-design/SKILL.md"))
	s7 := design[strings.Index(design, "7. **No `<epic>-impl` epic"):strings.Index(design, "8. **No `<epic>-impl` branch**")]
	if !strings.Contains(s7, rule) || !strings.Contains(s7, "follow [gx-to-tickets]") || !strings.Contains(s7, "inline") {
		t.Error("gx-design step 7 must run gx-to-tickets inline and carry the no-subagent rule")
	}
	if strings.Contains(s7, "gx tickets draft") {
		t.Error("gx-design step 7 names the draft command directly")
	}
}

// Spec "Skill size": the skill stays below 528 lines.
func TestToTicketsSkillSize(t *testing.T) {
	const limit = 528
	if got := strings.Count(readFile(t, "gx-to-tickets/SKILL.md"), "\n"); got >= limit {
		t.Errorf("gx-to-tickets/SKILL.md has %d lines, want < %d", got, limit)
	}
}

// The dry-run instruction sits in step 5 (trigger 3 needs it) and the writer
// lines use the wording the trace script's writers_wait bucket matches.
func TestToTicketsDryRunAndWriterLines(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	i := strings.Index(text, "### 5.")
	j := strings.Index(text, "### 6.")
	if i < 0 || j < i {
		t.Fatal("gx-to-tickets/SKILL.md steps 5, 6 not found in order")
	}
	if got := strings.Count(text[i:j], "--dry-run` first: trigger 3 needs its output"); got != 1 {
		t.Errorf("step 5 has %d dry-run instructions, want 1", got)
	}
	for _, anchor := range []string{"description that starts with `writer N`", "`writer N finished`"} {
		if got := strings.Count(text, anchor); got != 1 {
			t.Errorf("gx-to-tickets/SKILL.md has %d copies of %q, want 1", got, anchor)
		}
	}
}

// The split limit lives in two places: the skill text and the Go constant.
// This test keeps them equal.
func TestToTicketsSplitLimit(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	m := regexp.MustCompile(`more than (\d+)K tokens`).FindStringSubmatch(text)
	if m == nil {
		t.Fatal("gx-to-tickets/SKILL.md has no \"more than <N>K tokens\" split limit")
	}
	k, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := k * 1000; got != ticketspec.SplitLimitTokens {
		t.Errorf("skill says %dK, ticketspec.SplitLimitTokens is %d", k, ticketspec.SplitLimitTokens)
	}
}
