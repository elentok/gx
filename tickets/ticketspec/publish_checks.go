package ticketspec

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// PublishShapeMisses returns the mechanical checks an implement ticket fails
// before publish: both required sections present (Test seams non-empty) and the
// window at or under SplitLimitTokens. Other ticket types are exempt.
func PublishShapeMisses(ticketType, body string, window int) []string {
	if ticketType != "implement" {
		return nil
	}
	var misses []string
	if !schema.HasSection(body, "Test seams") {
		misses = append(misses, "no ## Test seams section")
	} else if schema.Section(body, "Test seams") == "" {
		misses = append(misses, "## Test seams is empty (write the seams, or `none — <reason>`)")
	}
	if !schema.HasSection(body, "Where to look") {
		misses = append(misses, "no ## Where to look section")
	}
	if window > SplitLimitTokens {
		misses = append(misses, fmt.Sprintf("expected_context_window %d is over the split limit %d", window, SplitLimitTokens))
	}
	return misses
}

var (
	groupHeadingRE = regexp.MustCompile(`^###\s+From\s+(\S+)\s*$`)
	checkboxRE     = regexp.MustCompile(`^\s*- \[[ xX]\]`)
)

// GroupCriteriaMisses returns one miss per member of from that has no
// "### From <address>" group with a checkbox under "## Acceptance criteria".
// A list of fewer than two members is not a group and has no misses. canon
// maps an address to its canonical form; members and headings are compared
// after it, so a short heading matches a canonical member.
func GroupCriteriaMisses(from []string, body string, canon func(string) string) []string {
	if len(from) < 2 {
		return nil
	}
	hasHeading := map[string]bool{}
	hasItem := map[string]bool{}
	current := ""
	for line := range strings.SplitSeq(schema.Section(body, "Acceptance criteria"), "\n") {
		if m := groupHeadingRE.FindStringSubmatch(line); m != nil {
			current = canon(m[1])
			hasHeading[current] = true
		} else if strings.HasPrefix(line, "#") {
			current = ""
		} else if current != "" && checkboxRE.MatchString(line) {
			hasItem[current] = true
		}
	}
	var misses []string
	for _, member := range from {
		switch c := canon(member); {
		case !hasHeading[c]:
			misses = append(misses, fmt.Sprintf("no `### From %s` group under ## Acceptance criteria", member))
		case !hasItem[c]:
			misses = append(misses, fmt.Sprintf("`### From %s` group has no `- [ ]` item", member))
		}
	}
	return misses
}
