package recovery

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/elentok/gx/tickets/schema"
)

// OutsideTextLine renders text from outside gx as the one line "- label: value",
// with value cut to maxRunes runes. The line starts with the label, so the value
// cannot open a heading, a section, a code fence or a dedupe line.
func OutsideTextLine(label, value string, maxRunes int) string {
	if runes := []rune(value); len(runes) > maxRunes {
		value = string(runes[:max(maxRunes, 0)])
	}
	return "- " + oneLine(label) + ": " + oneLine(value)
}

// oneLine turns every line break and control character into a space.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return ' '
		}
		return r
	}, s)
}

func dedupeLine(key string) string { return "Dedupe key: " + oneLine(key) }

// hasDedupeLine reports whether a whole line of body's Evidence section names key.
func hasDedupeLine(body, key string) bool {
	return slices.Contains(strings.Split(schema.Section(body, "Evidence"), "\n"), dedupeLine(key))
}

// countRepeat notes one repeat seen in source on date: it adds that pair's line
// to Evidence, or raises the count on the line already there.
func countRepeat(body, source, date string) string {
	seen := fmt.Sprintf("Seen again in %s on %s", oneLine(source), date)
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(seen) + `(?: \((\d+) times\))?\.$`)
	lines := strings.Split(body, "\n")
	inEvidence := false
	for i, line := range lines {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			inEvidence = strings.TrimSpace(heading) == "Evidence"
			continue
		}
		m := pattern.FindStringSubmatch(line)
		if !inEvidence || m == nil {
			continue
		}
		count := 1
		if m[1] != "" {
			count, _ = strconv.Atoi(m[1])
		}
		lines[i] = fmt.Sprintf("%s (%d times).", seen, count+1)
		return strings.Join(lines, "\n")
	}
	return schema.AppendToSection(body, "Evidence", seen+".")
}
