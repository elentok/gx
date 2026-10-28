package cmd

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsFollowUpSearch prints one line per open or archived follow-up of
// project whose title or ## Problem contains any of words (case-insensitive),
// so a dedupe reads a few hits instead of two whole listings. An archived hit
// also shows its close reason. An empty project means the cwd project.
func runTicketsFollowUpSearch(d deps, project string, words []string, w io.Writer) error {
	words = searchWords(words)
	if len(words) == 0 {
		return fmt.Errorf("follow-up search needs at least one word")
	}
	root, project, err := resolveProject(d, project)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	for _, name := range []string{followUpsEpic, followUpsArchivedEpic} {
		for _, e := range epics {
			if e.Name != name {
				continue
			}
			for _, t := range e.Tickets {
				haystack := strings.ToLower(t.Title + "\n" + schema.Section(t.Body, "Problem"))
				if !containsAny(haystack, words) {
					continue
				}
				a := tickets.Address{Project: project, Epic: name, ID: t.Identifier}
				line := fmt.Sprintf("%s  %s  %s", a, t.Status, t.Title)
				if name == followUpsArchivedEpic {
					if reason := closeReason(t.Body); reason != "" {
						line += "  (" + reason + ")"
					}
				}
				fmt.Fprintln(w, line)
			}
		}
	}
	return nil
}

func searchWords(words []string) []string {
	var out []string
	for _, w := range words {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

var closeReasonRe = regexp.MustCompile(`(?m)^- reason: (\S+)`)

// closeReason reads the reason runTicketsClose recorded in ## Resolution.
func closeReason(body string) string {
	if m := closeReasonRe.FindStringSubmatch(schema.Section(body, "Resolution")); m != nil {
		return m[1]
	}
	return ""
}
