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
	for _, name := range []string{"prefactor-tickets.md", "wide-refactors.md"} {
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
