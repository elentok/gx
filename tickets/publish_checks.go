package tickets

import (
	"fmt"
	"strings"
)

// sectionContent returns the lines under the `## heading` line up to the next
// heading, and whether the heading exists.
func sectionContent(body, heading string) (content string, found bool) {
	var lines []string
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if found && strings.HasPrefix(trimmed, "#") {
			break
		}
		if !found {
			found = trimmed == "## "+heading
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), found
}

// PublishShapeMisses returns the mechanical checks an implement ticket fails
// before publish: both required sections present (Test seams non-empty) and the
// window at or under SplitLimitTokens. Other ticket types are exempt.
func PublishShapeMisses(ticketType, body string, window int) []string {
	if ticketType != "implement" {
		return nil
	}
	var misses []string
	if content, found := sectionContent(body, "Test seams"); !found {
		misses = append(misses, "no ## Test seams section")
	} else if content == "" {
		misses = append(misses, "## Test seams is empty (write the seams, or `none — <reason>`)")
	}
	if _, found := sectionContent(body, "Where to look"); !found {
		misses = append(misses, "no ## Where to look section")
	}
	if window > SplitLimitTokens {
		misses = append(misses, fmt.Sprintf("expected_context_window %d is over the split limit %d", window, SplitLimitTokens))
	}
	return misses
}
