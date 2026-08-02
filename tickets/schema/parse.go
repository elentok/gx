package schema

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseTicket reads path and returns its metadata as a Ticket. The file must
// open with a "---" delimited YAML frontmatter block (see splitFrontmatter);
// a file without one fails with an error rather than being parsed some other
// way. A parsed ticket is run through Validate before being returned.
func ParseTicket(path string) (Ticket, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Ticket{}, fmt.Errorf("reading ticket file %s: %w", path, err)
	}

	return ParseTicketFromRaw(string(raw), path)
}

// ParseTicketFromRaw is ParseTicket's logic over an already-read file, for
// callers (e.g. tickets.Load) that need to derive both the typed Ticket and
// the raw body (via ParseBody) from one read.
func ParseTicketFromRaw(raw, path string) (Ticket, error) {
	yamlPart, _, hasFM := splitFrontmatter(raw)
	if !hasFM {
		return Ticket{}, fmt.Errorf("ticket %s has no frontmatter block: a \"---\" delimited YAML header is required", path)
	}

	var wire ticketYAML
	if err := yaml.Unmarshal([]byte(yamlPart), &wire); err != nil {
		return Ticket{}, fmt.Errorf("parsing frontmatter in %s: %w", path, err)
	}

	t := wire.toTicket()
	if err := Validate(t); err != nil {
		return Ticket{}, fmt.Errorf("invalid ticket %s: %w", path, err)
	}
	return t, nil
}

// ParseBody returns raw's markdown body: the content after the frontmatter
// block (schema.Ticket has no Body field of its own — see ticket.go's
// package doc — so this stays a standalone helper alongside
// ParseTicketFromRaw rather than a field on the returned Ticket). Callers
// only reach this after ParseTicketFromRaw has already confirmed raw has a
// frontmatter block; raw is returned unchanged in the (unreachable in
// practice) case where it doesn't.
func ParseBody(raw string) string {
	_, body, hasFM := splitFrontmatter(raw)
	if hasFM {
		return body
	}
	return raw
}
