package recovery

import (
	"fmt"
	"os"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// VerifyEveryStory opens every verify relaunch note: the lists adjust the
// scope, they never narrow it to the listed stories.
const VerifyEveryStory = "verify every story in the ticket; the lists below only adjust that"

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
	body := schema.ParseBody(string(raw))
	entries := schema.ParseVerified(body)
	units := schema.ParseStories(body)
	if len(entries) == 0 && len(units) == 0 {
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
	// With a ## Stories section the ticket owns the unit list, so a trimmed
	// ## Verified cannot shrink what is re-checked.
	if len(units) > 0 {
		order = units
	}
	for _, s := range order {
		e, ok := latest[s]
		switch {
		case !ok:
			recheck = append(recheck, "- "+s)
		case e.Counts(head):
			skip = append(skip, fmt.Sprintf("- %s (%s @ %s)", e.Story, e.Verdict, e.Commit))
		default:
			recheck = append(recheck, fmt.Sprintf("- %s (%s @ %s)", e.Story, e.Verdict, e.Commit))
		}
	}
	var b strings.Builder
	b.WriteString(VerifyEveryStory + "\n")
	fmt.Fprintf(&b, "This is a relaunch. HEAD is %s.", head)
	if len(entries) > 0 {
		b.WriteString(" The ticket's ## Verified section records earlier verdicts.")
	}
	b.WriteString("\n")
	if len(units) > 0 {
		fmt.Fprintf(&b, "Write each ## Verified line as `%s`, with the unit key exactly as in ## Stories: %s\n",
			schema.VerifiedLineFormat, strings.Join(units, "; "))
	}
	if len(skip) > 0 {
		fmt.Fprintf(&b, "Skip these stories, checked at HEAD:\n%s\n", strings.Join(skip, "\n"))
	}
	if len(recheck) > 0 {
		fmt.Fprintf(&b, "Re-check these stories, recorded at another commit:\n%s\n", strings.Join(recheck, "\n"))
	}
	return strings.TrimSpace(b.String())
}
