package schema

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Optional, additive body sections: a ticket without them parses as before.
const (
	VerifiedHeading         = "Verified"
	IterationSummaryHeading = "Iteration summaries"

	historyMarker = " [history]"
)

// OptionalSections are headings a ticket may lack until the first entry is
// written; reading one that is missing yields nothing instead of an error.
var OptionalSections = []string{IterationSummaryHeading}

// IsOptionalSection reports whether heading is on OptionalSections.
func IsOptionalSection(heading string) bool {
	return slices.Contains(OptionalSections, heading)
}

// VerifiedEntry is one "## Verified" bullet: a story, the verdict reached and
// the commit that was checked. History entries predate a Start over.
type VerifiedEntry struct {
	Story   string
	Verdict string
	Commit  string
	History bool
}

// Counts reports whether the entry still stands at head: only a live entry
// whose commit equals the worktree HEAD counts.
func (e VerifiedEntry) Counts(head string) bool {
	return !e.History && e.Commit != "" && e.Commit == head
}

func (e VerifiedEntry) line() string {
	l := "- " + oneLine(e.Story) + " — " + oneLine(e.Verdict) + " @ " + oneLine(e.Commit)
	if e.History {
		l += historyMarker
	}
	return l
}

var verifiedLine = regexp.MustCompile(`^- (.*) — (\S+) @ (\S+)( \[history\])?$`)

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// AppendVerified adds e to body's "## Verified" section, creating it if needed.
func AppendVerified(body string, e VerifiedEntry) string {
	return appendEntry(body, VerifiedHeading, e.line(), "\n")
}

// ParseVerified returns the entries of body's "## Verified" section in order;
// a body without one yields none.
func ParseVerified(body string) []VerifiedEntry {
	var out []VerifiedEntry
	for line := range strings.SplitSeq(Section(body, VerifiedHeading), "\n") {
		m := verifiedLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		out = append(out, VerifiedEntry{Story: m[1], Verdict: m[2], Commit: m[3], History: m[4] != ""})
	}
	return out
}

// MarkVerifiedHistory flags every Verified entry as history, so none counts
// after a Start over. A body without the section is returned unchanged.
func MarkVerifiedHistory(body string) string {
	entries := ParseVerified(body)
	if len(entries) == 0 {
		return body
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		e.History = true
		lines[i] = e.line()
	}
	return SetSection(body, VerifiedHeading, strings.Join(lines, "\n"))
}

// IterationSummary is one "## Iteration summaries" entry.
type IterationSummary struct {
	SessionID string
	Time      time.Time
	Label     string // agent or gx
	Did       string
	WentWrong string
	Decisions string
	Left      string
}

var summaryFields = []struct {
	label string
	field func(*IterationSummary) *string
}{
	{"Did", func(s *IterationSummary) *string { return &s.Did }},
	{"Went wrong", func(s *IterationSummary) *string { return &s.WentWrong }},
	{"Decisions", func(s *IterationSummary) *string { return &s.Decisions }},
	{"Left", func(s *IterationSummary) *string { return &s.Left }},
}

func (s IterationSummary) block() string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s — %s — %s\n", oneLine(s.SessionID), s.Time.UTC().Format(time.RFC3339), oneLine(s.Label))
	for _, f := range summaryFields {
		fmt.Fprintf(&b, "\n- %s: %s", f.label, oneLine(*f.field(&s)))
	}
	return b.String()
}

// AppendIterationSummary adds s to body's "## Iteration summaries" section,
// creating it if needed.
func AppendIterationSummary(body string, s IterationSummary) string {
	return appendEntry(body, IterationSummaryHeading, s.block(), "\n\n")
}

// ParseIterationSummaries returns the entries of body's "## Iteration
// summaries" section in order; a body without one yields none.
func ParseIterationSummaries(body string) []IterationSummary {
	var out []IterationSummary
	var cur *IterationSummary
	for line := range strings.SplitSeq(Section(body, IterationSummaryHeading), "\n") {
		if head, ok := strings.CutPrefix(line, "### "); ok {
			parts := strings.SplitN(head, " — ", 3)
			if len(parts) != 3 {
				cur = nil
				continue
			}
			t, _ := time.Parse(time.RFC3339, parts[1])
			out = append(out, IterationSummary{SessionID: parts[0], Time: t, Label: parts[2]})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil {
			continue
		}
		for _, f := range summaryFields {
			if v, ok := strings.CutPrefix(line, "- "+f.label+":"); ok {
				*f.field(cur) = strings.TrimSpace(v)
			}
		}
	}
	return out
}

// appendEntry appends entry to body's "## heading" section, creating the
// section when body has none. sep separates entries within the section.
func appendEntry(body, heading, entry, sep string) string {
	if !HasSection(body, heading) {
		return SetSection(body, heading, entry)
	}
	existing := Section(body, heading)
	if existing == "" {
		return SetSection(body, heading, entry)
	}
	return SetSection(body, heading, existing+sep+entry)
}
