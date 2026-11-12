package skills

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets/ticketspec"
)

func TestToTicketsPrefactorChecks(t *testing.T) {
	text := normalizeSpace(readFile(t, "gx-to-tickets/SKILL.md"))
	for _, anchor := range []string{"#### Prefactor tickets", "`sizeguard`", "gopls", "the export decision"} {
		if !strings.Contains(text, normalizeSpace(anchor)) {
			t.Errorf("gx-to-tickets/SKILL.md is missing %q", anchor)
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
