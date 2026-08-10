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
// the raw body (via ParseBody) from one read. It accepts only the
// post-migration shape: a retired `children` field is rejected here rather
// than ignored, so the pre-contraction shape can't quietly round-trip through
// a reader that simply doesn't look at it.
func ParseTicketFromRaw(raw, path string) (Ticket, error) {
	r, err := ParseTicketRaw(raw, path)
	if err != nil {
		return Ticket{}, err
	}
	if r.HasChildren {
		return Ticket{}, fmt.Errorf("invalid ticket %s: children: retired field, run `gx tickets migrate` (a ticket's children are derived from its forks' parent)", path)
	}
	if err := Validate(r.Ticket); err != nil {
		return Ticket{}, fmt.Errorf("invalid ticket %s: %w", path, err)
	}
	return r.Ticket, nil
}

// RawTicket is one ticket file as it sits on disk: the typed Ticket plus what
// the frontmatter carried that Ticket itself no longer has a home for. It
// exists so the file is decoded exactly once — the loader's rejection of a
// retired field and migration's report of the same field are two readings of
// one parse, not two parses that can disagree.
type RawTicket struct {
	Ticket Ticket
	// HasChildren reports the retired `children` key's presence, whatever
	// value shape it had.
	HasChildren bool
}

// ParseTicketRaw is ParseTicketFromRaw without the trailing Validate call and
// without the retired-field rejection — for a caller (`gx tickets migrate`)
// that must inspect and repair a ticket exactly as it sits on disk, including
// a pre-refactor shape (e.g. a missing status:) that would fail Validate
// before the repair ever runs.
func ParseTicketRaw(raw, path string) (RawTicket, error) {
	yamlPart, _, hasFM := splitFrontmatter(raw)
	if !hasFM {
		return RawTicket{}, fmt.Errorf("ticket %s has no frontmatter block: a \"---\" delimited YAML header is required", path)
	}

	var wire ticketYAML
	if err := yaml.Unmarshal([]byte(yamlPart), &wire); err != nil {
		return RawTicket{}, fmt.Errorf("parsing frontmatter in %s: %w", path, err)
	}

	return RawTicket{Ticket: wire.toTicket(), HasChildren: wire.hasChildren()}, nil
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
