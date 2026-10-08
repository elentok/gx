package schema

import "strings"

// SetSection returns body with the "## heading" section's content replaced,
// or appended as a new section when body has none. A section runs to the next
// "## " line; lines inside code fences never count as headings.
func SetSection(body, heading, content string) string {
	section := "## " + heading + "\n\n" + strings.Trim(content, "\n") + "\n"

	lines := strings.SplitAfter(body, "\n")
	start, end := -1, len(lines)
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "## ") {
			continue
		}
		if start >= 0 {
			end = i
			break
		}
		if strings.TrimSpace(line[3:]) == heading {
			start = i
		}
	}

	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + section
	}
	rest := strings.Join(lines[end:], "")
	if rest != "" {
		section += "\n"
	}
	return strings.Join(lines[:start], "") + section + rest
}
