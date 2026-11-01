package tickets

import (
	"fmt"

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
