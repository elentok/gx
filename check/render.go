package check

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/brief"
)

// fewFailures is how many failures a kind lists without naming a dominant error.
const fewFailures = 3

// dominantCause returns the most common non-empty line and how often it occurs;
// the earliest wins a tie.
func dominantCause(causes []string) (string, int) {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, c := range causes {
		if c == "" {
			continue
		}
		counts[c]++
		if counts[c] > bestN {
			best, bestN = c, counts[c]
		}
	}
	return best, bestN
}

// Render is the text `gx check` prints and writes to the summary file: the
// summary path, one line per kind, the log and tail of each failing kind, then
// the overall result.
func (r Result) Render() string {
	var b strings.Builder
	fmt.Fprintln(&b, r.Summary)
	for _, k := range r.Kinds {
		detail := k.Reason
		if k.Status != StatusSkip {
			detail = fmt.Sprintf("%.2fs", k.Duration.Seconds())
		}
		fmt.Fprintf(&b, "%-4s  %-9s  %s\n", k.Status, k.Kind, detail)
		for _, n := range k.Notes {
			fmt.Fprintf(&b, "      %s\n", n)
		}
		for _, w := range k.Warnings {
			fmt.Fprintf(&b, "      warning: %s\n", w)
		}
	}
	failed := 0
	for _, k := range r.Kinds {
		if k.Status != StatusFail {
			continue
		}
		failed++
		fmt.Fprintf(&b, "\n%s log: %s\n", k.Kind, k.Log)
		if line, n := dominantCause(k.Causes); len(k.Failures) > fewFailures && n > 1 {
			fmt.Fprintf(&b, "  most common error (%d of %d failures): %s\n", n, len(k.Failures), line)
		}
		for _, name := range k.Failures {
			fmt.Fprintf(&b, "  %s\n", name)
		}
		tail := brief.TrimTail(k.Tail)
		if k.Kind == "typecheck" {
			tail = brief.DiagnosticLines(tail)
			tail = tail[max(0, len(tail)-tailLines):]
		}
		for _, line := range tail {
			fmt.Fprintf(&b, "  %s\n", line)
		}
		if k.Kind == "typecheck" {
			fmt.Fprintf(&b, "  rerun: gx check --kind typecheck\n")
		}
	}
	switch r.Exit {
	case ExitFail:
		fmt.Fprintf(&b, "\nFAIL (%d failed)\n", failed)
	case ExitNone:
		fmt.Fprintln(&b, "\nno applicable checks")
	default:
		fmt.Fprintln(&b, "\nPASS")
	}
	return b.String()
}
