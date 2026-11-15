package schema

import (
	"fmt"
	"regexp"
	"strings"
)

// StoriesHeading is the verify ticket section that lists its units. The unit
// key written there is the same string a "## Verified" entry uses.
const StoriesHeading = "Stories"

// storyLine accepts "- Story <n>" and "- Mock states: <name>"; any other line is
// not a unit, in the same strict way as verifiedLine.
var storyLine = regexp.MustCompile(`^- (Story [1-9][0-9]*|Mock states: \S.*)$`)

// StoryKey is the unit key of numbered story n.
func StoryKey(n int) string { return fmt.Sprintf("Story %d", n) }

// MockStatesKey is the unit key of the Mock states line for state.
func MockStatesKey(state string) string { return "Mock states: " + oneLine(state) }

// StoriesSection renders keys as the body of a "## Stories" section.
func StoriesSection(keys []string) string {
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = "- " + k
	}
	return strings.Join(lines, "\n")
}

// ParseStories returns the unit keys of body's "## Stories" section in order;
// a body without one yields none.
func ParseStories(body string) []string {
	var out []string
	for line := range strings.SplitSeq(Section(body, StoriesHeading), "\n") {
		if m := storyLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}
