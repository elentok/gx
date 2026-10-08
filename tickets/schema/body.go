package schema

import (
	"fmt"
	"strings"
	"time"
)

// bodySection is one "## Heading" block of a ticket body: the heading line
// itself plus everything up to (not including) the next top-level heading.
type bodySection struct {
	heading string
	content string
}

// splitBodySections splits body into the free text before its first "## "
// heading (preamble) and the ordered list of heading blocks that follow.
// Rejoining preamble and sections with joinBodySections reproduces body
// exactly when neither is modified.
func splitBodySections(body string) (preamble string, sections []bodySection) {
	lines := strings.Split(body, "\n")
	first := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			first = i
			break
		}
	}
	if first == -1 {
		return body, nil
	}
	preamble = strings.Join(lines[:first], "\n")
	i := first
	for i < len(lines) {
		heading := lines[i]
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "## ") {
			j++
		}
		sections = append(sections, bodySection{heading: heading, content: strings.Join(lines[i+1:j], "\n")})
		i = j
	}
	return preamble, sections
}

// Section returns the trimmed content under body's "## heading" (heading given
// without the "## "), or "" when there is none.
func Section(body, heading string) string {
	_, sections := splitBodySections(body)
	for _, s := range sections {
		if strings.TrimSpace(strings.TrimPrefix(s.heading, "## ")) == heading {
			return strings.TrimSpace(s.content)
		}
	}
	return ""
}

// joinBodySections is splitBodySections's inverse.
func joinBodySections(preamble string, sections []bodySection) string {
	parts := []string{}
	if preamble != "" {
		parts = append(parts, preamble)
	}
	for _, s := range sections {
		parts = append(parts, s.heading+"\n"+s.content)
	}
	return strings.Join(parts, "\n")
}

// DemoteSection moves body's heading section (if any) into a dated
// sub-entry appended under "## Comments" — creating that heading if it
// doesn't exist yet — and removes heading itself. Replacing rather than
// appending a fresh copy of heading on top is what fixes the live stacking
// bug: a body with no such section is returned unchanged, so retiring the
// same section twice in a row (e.g. two claims, or a claim after an
// automatic unpark already retired it) is a no-op here.
func DemoteSection(body, heading string, now time.Time) string {
	preamble, sections := splitBodySections(body)

	idx := -1
	for i, s := range sections {
		if s.heading == heading {
			idx = i
			break
		}
	}
	if idx == -1 {
		return body
	}

	reason := strings.TrimSpace(sections[idx].content)
	entry := fmt.Sprintf("**%s** — retired from `%s`:\n\n%s\n", now.Format("2006-01-02"), heading, reason)
	sections = append(sections[:idx], sections[idx+1:]...)
	sections = appendCommentsEntry(sections, entry)

	return joinBodySections(preamble, sections)
}

// AppendComment appends entry as a dated sub-entry under body's "## Comments"
// heading, creating the heading if it doesn't exist yet.
func AppendComment(body, entry string) string {
	preamble, sections := splitBodySections(body)
	sections = appendCommentsEntry(sections, entry)
	return joinBodySections(preamble, sections)
}

// appendCommentsEntry appends entry under sections' "## Comments" heading,
// creating that heading if it doesn't exist yet — the shared write DemoteSection
// and AppendComment both funnel through.
func appendCommentsEntry(sections []bodySection, entry string) []bodySection {
	for i, s := range sections {
		if s.heading == "## Comments" {
			existing := strings.TrimRight(s.content, "\n")
			if strings.TrimSpace(existing) == "" {
				sections[i].content = "\n" + entry
			} else {
				sections[i].content = existing + "\n\n" + entry
			}
			return sections
		}
	}
	return append(sections, bodySection{heading: "## Comments", content: "\n" + entry})
}
