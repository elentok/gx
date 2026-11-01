package tickets

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// BundledThreshold is the pair a ticket must reach on both counts to get the
// "consider splitting" advisory. Retune after 2 or 3 epics.
var BundledThreshold = struct{ Criteria, Files int }{Criteria: 5, Files: 5}

// criteriaBoilerplate are acceptance bullets that every ticket carries and that
// say nothing about size. Matched on the lowercased bullet with markup removed.
var criteriaBoilerplate = []string{
	"tests pass",
	"all tests pass",
	"make verify is green",
	"gx check is green",
	"gx check passes",
}

var bulletRe = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?(.*)$`)

// BundledCounts returns the number of acceptance criteria (bullets and numbered
// items under `## Acceptance criteria`, minus boilerplate) and the number of
// distinct file paths named in `## Where to look` and `## Test seams`.
func BundledCounts(body string, topDirs []string) (criteria, files int) {
	for line := range strings.SplitSeq(schema.Section(body, "Acceptance criteria"), "\n") {
		m := bulletRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.ToLower(strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(m[1]), "`", ""), ". "))
		if !slices.Contains(criteriaBoilerplate, text) {
			criteria++
		}
	}

	seen := map[string]bool{}
	for _, c := range ExtractClaims(body, topDirs) {
		if c.Section != "Where to look" && c.Section != "Test seams" {
			continue
		}
		switch c.Kind {
		case ClaimPath, ClaimFileFunc, ClaimLineRef:
			seen[c.File] = true
		}
	}
	return criteria, len(seen)
}

// BundledAdvisory returns the advisory line for an implement ticket that reaches
// both thresholds, or "".
func BundledAdvisory(slug, ticketType, body string, topDirs []string) string {
	if ticketType != "implement" {
		return ""
	}
	criteria, files := BundledCounts(body, topDirs)
	if criteria < BundledThreshold.Criteria || files < BundledThreshold.Files {
		return ""
	}
	return fmt.Sprintf("%s: %d acceptance criteria and %d files, consider splitting", slug, criteria, files)
}
