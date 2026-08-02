package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/internal/legacyparse"
	"gopkg.in/yaml.v3"
)

// ParseTicket reads path and returns its metadata as a Ticket, handling
// both a "---" delimited frontmatter block and the legacy bold-line format
// (see parseOldFormat). Frontmatter tickets are run through Validate before
// being returned; old-format tickets are not (see parseOldFormat's doc
// comment for why).
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
		return parseOldFormat(raw, path), nil
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
// block for a new-format ticket, or legacyparse.Parse's metadata-stripped
// body for an old-format one (schema.Ticket has no Body field of its own —
// see ticket.go's package doc — so this stays a standalone helper alongside
// ParseTicketFromRaw rather than a field on the returned Ticket).
func ParseBody(raw string) string {
	_, body, hasFM := splitFrontmatter(raw)
	if hasFM {
		return body
	}
	return legacyparse.Parse(raw).Body
}

// filenameIDRe pulls the ticket ID off a "NN[a]-slug.md" filename, mirroring
// tickets.parseTicketFilename's numeric+optional-letter convention.
var filenameIDRe = regexp.MustCompile(`^(\d+[a-z]?)-`)

// extraFieldRe matches the old-format bold-line fields that
// tickets.ParseTicket doesn't already extract (Type/Blocked by/Status).
// "**" is stripped from the line before matching, same as tickets.ParseTicket,
// so "**Split:** 04b" and "Split: 04b" parse the same way.
var extraFieldRe = regexp.MustCompile(`(?i)^(Split|Code-review fixes|Context window|Following-up):\s*(.*)$`)

// trailingAnnotationRe strips a trailing parenthetical or em-dash aside from
// a field value, e.g. "06b — this ticket landed the data side only" -> "06b",
// mirroring tickets.go's blockedByParentheticalRe/statusEmDashAnnotationRe.
var trailingAnnotationRe = regexp.MustCompile(`\s*(\(.*\)|—.*)\s*$`)

var ticketTokenRe = regexp.MustCompile(`\d+[a-zA-Z]*`)

// extractTicketTokens pulls ticket-ID-shaped tokens (e.g. "04b") out of a
// Split:/Following-up: value, ignoring any trailing prose annotation.
func extractTicketTokens(value string) []string {
	return ticketTokenRe.FindAllString(trailingAnnotationRe.ReplaceAllString(value, ""), -1)
}

var firstWordRe = regexp.MustCompile(`^\S+`)

// parseOldFormat maps a legacy bold-line ticket file onto the new Ticket
// struct: legacyparse.Parse already extracts Type/Blocked by/Status, and
// this adds the fields it doesn't cover (Split, Code-review fixes, Context
// window -> ActualContextWindow, Following-up -> SplitFrom), per the
// field-name mapping in ticket 01b. The ticket ID isn't part of any
// bold-line field, so it's derived from the filename instead, matching
// tickets.parseTicketFilename's convention.
//
// Validate is deliberately not run here: old files predate the schema (a
// missing Status: line, or a synonym like "resolved", is valid under the
// legacy format but not the new enum), and migration ticket 03 is what
// reconciles that, not this fallback path.
func parseOldFormat(raw, path string) Ticket {
	legacy := legacyparse.Parse(raw)

	t := Ticket{
		ID:        deriveIDFromFilename(path),
		Status:    Status(legacy.Status),
		Type:      TicketType(legacy.Type),
		BlockedBy: stringsToIDs(legacy.BlockedBy),
	}

	for _, line := range strings.Split(raw, "\n") {
		m := extraFieldRe.FindStringSubmatch(strings.ReplaceAll(line, "**", ""))
		if m == nil {
			continue
		}
		key := strings.ToLower(m[1])
		value := strings.TrimSpace(m[2])
		switch key {
		case "split":
			t.Split = stringsToIDs(extractTicketTokens(value))
		case "code-review fixes":
			t.CodeReviewFixes = firstWordRe.FindString(value)
		case "context window":
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				t.ActualContextWindow = n
			}
		case "following-up":
			if ids := extractTicketTokens(value); len(ids) > 0 {
				id := TicketID(ids[0])
				t.SplitFrom = &id
			}
		}
	}

	return t
}

func deriveIDFromFilename(path string) TicketID {
	m := filenameIDRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return ""
	}
	return TicketID(m[1])
}
