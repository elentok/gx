package ticketsflow

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsFollowUpSearch prints one line per open or archived follow-up of
// project whose title or ## Problem contains any of words (case-insensitive),
// so a dedupe reads a few hits instead of two whole listings. An archived hit
// also shows its close reason. Open hits print in full; archived hits come
// newest first, capped at maxArchivedHits unless all. No hit prints "no
// matches". An empty project means the cwd project.
func runTicketsFollowUpSearch(d Deps, project string, words, queries []string, all bool, w io.Writer) error {
	// Positional words form one query and each --query is another. Only a
	// lone positional query keeps the bare "no matches" line.
	var qs []string
	if len(words) > 0 {
		qs = append(qs, strings.Join(words, " "))
	}
	qs = append(qs, queries...)
	if len(qs) == 0 || len(searchWords(qs)) == 0 {
		return fmt.Errorf("follow-up search needs at least one word")
	}
	bare := len(queries) == 0
	root, project, err := ticketref.ResolveProject(d.Env, project)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	var open, archived []string
	hit := make([]bool, len(qs))
	for _, name := range []string{tickets.FollowUpsEpic, tickets.ArchivedFollowUpsEpic} {
		for _, e := range epics {
			if e.Name != name {
				continue
			}
			ts := append([]tickets.Ticket(nil), e.Tickets...)
			if name == tickets.ArchivedFollowUpsEpic {
				sort.SliceStable(ts, func(i, j int) bool { return newerID(ts[i].Identifier, ts[j].Identifier) })
			}
			for _, t := range ts {
				haystack := strings.ToLower(t.Title + "\n" + schema.Section(t.Body, "Problem"))
				matched := false
				for i, q := range qs {
					if containsAny(haystack, searchWords([]string{q})) {
						hit[i], matched = true, true
					}
				}
				if !matched {
					continue
				}
				a := tickets.Address{Project: project, Epic: name, ID: t.Identifier}
				line := fmt.Sprintf("%s  %s  %s", a, t.Status, t.Title)
				if name != tickets.ArchivedFollowUpsEpic {
					open = append(open, line)
					continue
				}
				if reason := closeReason(t.Body); reason != "" {
					line += "  (" + reason + ")"
				}
				archived = append(archived, line)
			}
		}
	}
	if bare && len(open)+len(archived) == 0 {
		fmt.Fprintln(w, "no matches")
		return nil
	}
	for _, line := range open {
		fmt.Fprintln(w, line)
	}
	shown := archived
	if !all && len(shown) > maxArchivedHits {
		shown = shown[:maxArchivedHits]
	}
	for _, line := range shown {
		fmt.Fprintln(w, line)
	}
	if left := len(archived) - len(shown); left > 0 {
		fmt.Fprintf(w, "%d more archived hits not shown; --all prints them\n", left)
	}
	if !bare {
		for i, q := range qs {
			if !hit[i] {
				fmt.Fprintf(w, "no matches: %s\n", q)
			}
		}
	}
	return nil
}

const maxArchivedHits = 10

// newerID reports whether id a sorts before b when newest comes first: IDs
// are zero-padded counters, so a longer one is larger.
func newerID(a, b string) bool {
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a > b
}

func searchWords(words []string) []string {
	var out []string
	for _, w := range words {
		out = append(out, strings.Fields(strings.ToLower(w))...)
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
