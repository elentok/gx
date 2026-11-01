package tickets

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	specItemRe = regexp.MustCompile(`^(\d+)\. `)
	// specHeadingRe matches any level-2 heading line.
	specHeadingRe = regexp.MustCompile(`^## `)
)

// SpecSection returns the lines under the `## heading` of spec, up to the next
// level-2 heading, with surrounding blank lines trimmed. ok is false when the
// heading is missing.
func SpecSection(spec, heading string) (string, bool) {
	lines := strings.Split(spec, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "## "+heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if specHeadingRe.MatchString(lines[i]) {
			end = i
			break
		}
	}
	return strings.Trim(strings.Join(lines[start:end], "\n"), "\n"), true
}

// continuation reads the lines after lines[i] that belong to the same block:
// indented lines, and blank lines followed by an indented line. It returns the
// index of the first line outside the block.
func continuation(lines []string, i int) int {
	j := i + 1
	for j < len(lines) {
		switch {
		case strings.HasPrefix(lines[j], " ") || strings.HasPrefix(lines[j], "\t"):
			j++
		case strings.TrimSpace(lines[j]) == "" && j+1 < len(lines) &&
			(strings.HasPrefix(lines[j+1], " ") || strings.HasPrefix(lines[j+1], "\t")):
			j++
		default:
			return j
		}
	}
	return j
}

// SpecListItem returns numbered item n of a section, word for word, with its
// indented continuation lines.
func SpecListItem(section string, n int) (string, bool) {
	lines := strings.Split(section, "\n")
	for i, l := range lines {
		m := specItemRe.FindStringSubmatch(l)
		if m == nil || m[1] != fmt.Sprint(n) {
			continue
		}
		return strings.Join(lines[i:continuation(lines, i)], "\n"), true
	}
	return "", false
}

// SpecMockLine returns the bullet of the section's `**Mock states**` list
// whose fields (split on commas and middle dots) include state, word for word.
func SpecMockLine(section, state string) (string, bool) {
	lines := strings.Split(section, "\n")
	inList := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "**Mock states**") {
			inList = true
			continue
		}
		if !inList || !strings.HasPrefix(l, "- ") {
			continue
		}
		end := continuation(lines, i)
		item := strings.Join(lines[i:end], "\n")
		flat := strings.Join(strings.Fields(strings.TrimPrefix(item, "- ")), " ")
		for _, f := range strings.FieldsFunc(flat, func(r rune) bool { return r == ',' || r == '·' }) {
			if strings.Trim(strings.TrimSpace(f), "`") == state {
				return item, true
			}
		}
		i = end - 1
	}
	return "", false
}

// EscapePlaceholders writes every `{{` as `\{{` so publish leaves quoted
// placeholders alone.
func EscapePlaceholders(s string) string {
	return strings.ReplaceAll(s, "{{", `\{{`)
}
