package check

import (
	"fmt"
	"strings"
)

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
	}
	failed := 0
	for _, k := range r.Kinds {
		if k.Status != StatusFail {
			continue
		}
		failed++
		fmt.Fprintf(&b, "\n%s log: %s\n", k.Kind, k.Log)
		for _, name := range k.Failures {
			fmt.Fprintf(&b, "  %s\n", name)
		}
		for _, line := range k.Tail {
			fmt.Fprintf(&b, "  %s\n", line)
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
