package skills

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/elentok/gx/tickets"
)

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
	if got := k * 1000; got != tickets.SplitLimitTokens {
		t.Errorf("skill says %dK, tickets.SplitLimitTokens is %d", k, tickets.SplitLimitTokens)
	}
}
