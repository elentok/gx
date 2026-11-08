package recovery

import (
	"fmt"
	"os"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// VerifyResumeNote is what a relaunched verify agent is told on top of the
// skill command: which recorded stories stand at head (skip them) and which
// do not (check them again). A ticket with no Verified entries gets no note.
func VerifyResumeNote(t tickets.Ticket, head string) string {
	if !t.IsVerify() {
		return ""
	}
	raw, err := os.ReadFile(t.Path)
	if err != nil {
		return ""
	}
	entries := schema.ParseVerified(schema.ParseBody(string(raw)))
	if len(entries) == 0 {
		return ""
	}
	// The latest entry per story wins.
	latest := map[string]schema.VerifiedEntry{}
	var order []string
	for _, e := range entries {
		if _, seen := latest[e.Story]; !seen {
			order = append(order, e.Story)
		}
		latest[e.Story] = e
	}
	var skip, recheck []string
	for _, s := range order {
		e := latest[s]
		line := fmt.Sprintf("- %s (%s @ %s)", e.Story, e.Verdict, e.Commit)
		if e.Counts(head) {
			skip = append(skip, line)
		} else {
			recheck = append(recheck, line)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "This is a relaunch. HEAD is %s. The ticket's ## Verified section records earlier verdicts.\n", head)
	if len(skip) > 0 {
		fmt.Fprintf(&b, "Skip these stories, checked at HEAD:\n%s\n", strings.Join(skip, "\n"))
	}
	if len(recheck) > 0 {
		fmt.Fprintf(&b, "Re-check these stories, recorded at another commit:\n%s\n", strings.Join(recheck, "\n"))
	}
	return strings.TrimSpace(b.String())
}
