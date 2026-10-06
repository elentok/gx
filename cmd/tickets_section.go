package cmd

import (
	"io"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// runTicketsSection replaces (or appends) the "## heading" section of path's
// ticket body with content, under the per-ticket lock, and prints the ticket's
// address.
func runTicketsSection(path, heading, content string, jsonOut bool, w io.Writer) error {
	err := schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		*body = setSection(*body, heading, content)
	})
	if err != nil {
		return err
	}
	pingServer(path)
	return printTicketRef(path, jsonOut, w)
}

// setSection returns body with the "## heading" section's content replaced,
// or appended as a new section when body has none. A section runs to the next
// "## " line; lines inside code fences never count as headings.
func setSection(body, heading, content string) string {
	section := "## " + heading + "\n\n" + strings.Trim(content, "\n") + "\n"

	lines := strings.SplitAfter(body, "\n")
	start, end := -1, len(lines)
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "## ") {
			continue
		}
		if start >= 0 {
			end = i
			break
		}
		if strings.TrimSpace(line[3:]) == heading {
			start = i
		}
	}

	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n" + section
	}
	rest := strings.Join(lines[end:], "")
	if rest != "" {
		section += "\n"
	}
	return strings.Join(lines[:start], "") + section + rest
}
