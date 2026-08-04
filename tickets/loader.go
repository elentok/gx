package tickets

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

var ticketFilenameRe = regexp.MustCompile(`^(\d+)([[:alpha:]]*)-(.+)\.md$`)

// Load reads a `.scratch/` directory from real disk into its epics/tickets.
// A missing directory is not an error: it returns a nil/empty slice, which
// renders the same empty state as a present-but-empty `.scratch/`.
func Load(scratchDir string) ([]Epic, error) {
	entries, err := os.ReadDir(scratchDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var epics []Epic
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		epics = append(epics, loadEpic(scratchDir, entry.Name()))
	}
	return epics, nil
}

func loadEpic(scratchDir, name string) Epic {
	epicPath := filepath.Join(scratchDir, name)
	epic := Epic{Name: name, Path: epicPath}

	if raw, err := os.ReadFile(filepath.Join(epicPath, "map.md")); err == nil {
		epic.IsMap = true
		epic.MapBody = string(raw)
	}

	issuesDir := filepath.Join(epicPath, "issues")
	issueEntries, err := os.ReadDir(issuesDir)
	if err != nil {
		return epic
	}

	for _, issueEntry := range issueEntries {
		if issueEntry.IsDir() {
			continue
		}
		number, identifier, title, ok := parseTicketFilename(issueEntry.Name())
		if !ok {
			continue
		}

		ticketPath := filepath.Join(issuesDir, issueEntry.Name())
		ticket := Ticket{
			Number:     number,
			Identifier: identifier,
			Title:      title,
			Path:       ticketPath,
		}

		raw, err := os.ReadFile(ticketPath)
		if err != nil {
			ticket.ReadErr = err.Error()
			epic.Tickets = append(epic.Tickets, ticket)
			continue
		}

		parsed, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
		if err != nil {
			ticket.ReadErr = err.Error()
			epic.Tickets = append(epic.Tickets, ticket)
			continue
		}

		ticket.Type = string(parsed.Type)
		ticket.BlockedBy = idsToStrings(parsed.BlockedBy)
		ticket.Split = idsToStrings(parsed.Split)
		ticket.SplitFrom = idToStringPtr(parsed.SplitFrom)
		ticket.Status = string(parsed.Status)
		ticket.Body = schema.ParseBody(string(raw))
		ticket.ActualContextWindow = parsed.ActualContextWindow
		ticket.ElapsedTime = parsed.ElapsedTime
		ticket.Commitless = parsed.Commitless
		epic.Tickets = append(epic.Tickets, ticket)
	}

	return epic
}

// parseTicketFilename splits a "NN[suffix]-<slug>.md" filename into its
// numeric sort key, full identifier, and a humanized title. The optional
// alphabetic suffix supports wayfinder's split tickets (for example 10a).
func parseTicketFilename(filename string) (number int, identifier, title string, ok bool) {
	m := ticketFilenameRe.FindStringSubmatch(filename)
	if m == nil {
		return 0, "", "", false
	}
	number, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", "", false
	}
	identifier = m[1] + m[2]
	return number, identifier, humanizeSlug(m[3]), true
}

func humanizeSlug(slug string) string {
	title := strings.ReplaceAll(slug, "-", " ")
	if title == "" {
		return title
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

// idsToStrings lowers schema.TicketIDs to plain strings for tickets.Ticket's
// BlockedBy field, which predates the schema package and is read directly by
// ralphloop/ui call sites that don't know about schema.TicketID.
func idsToStrings(ids []schema.TicketID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// idToStringPtr lowers an optional schema.TicketID to a plain *string, for
// tickets.Ticket's SplitFrom field (see idsToStrings).
func idToStringPtr(id *schema.TicketID) *string {
	if id == nil {
		return nil
	}
	s := string(*id)
	return &s
}
