package tickets

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
		raw, err := os.ReadFile(ticketPath)
		if err != nil {
			epic.Tickets = append(epic.Tickets, Ticket{
				Number:     number,
				Identifier: identifier,
				Title:      title,
				Path:       ticketPath,
				ReadErr:    err.Error(),
			})
			continue
		}

		ticket, _ := ParseTicket(string(raw))
		ticket.Number = number
		ticket.Identifier = identifier
		ticket.Title = title
		ticket.Path = ticketPath
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
