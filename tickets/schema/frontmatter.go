package schema

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ticketYAML is the on-disk YAML shape of a ticket's frontmatter block —
// separate from Ticket so the wire format (plain strings/slices) can diverge
// from the typed in-memory representation (TicketID, Status, etc.) without
// yaml struct tags leaking into schema's public API.
type ticketYAML struct {
	ID                    string   `yaml:"id"`
	Status                string   `yaml:"status"`
	BlockedBy             []string `yaml:"blocked_by,omitempty"`
	Split                 []string `yaml:"split,omitempty"`
	SplitFrom             string   `yaml:"split_from,omitempty"`
	Type                  string   `yaml:"type"`
	CodeReviewFixes       string   `yaml:"code_review_fixes,omitempty"`
	ExpectedContextWindow int      `yaml:"expected_context_window,omitempty"`
	ActualContextWindow   int      `yaml:"actual_context_window,omitempty"`
	ElapsedTime           int      `yaml:"elapsed_time,omitempty"`
	Compactions           int      `yaml:"compactions,omitempty"`
	Commitless            bool     `yaml:"commitless,omitempty"`
}

func (w ticketYAML) toTicket() Ticket {
	t := Ticket{
		ID:                    TicketID(w.ID),
		Status:                Status(w.Status),
		BlockedBy:             stringsToIDs(w.BlockedBy),
		Split:                 stringsToIDs(w.Split),
		Type:                  TicketType(w.Type),
		CodeReviewFixes:       w.CodeReviewFixes,
		ExpectedContextWindow: w.ExpectedContextWindow,
		ActualContextWindow:   w.ActualContextWindow,
		ElapsedTime:           w.ElapsedTime,
		Compactions:           w.Compactions,
		Commitless:            w.Commitless,
	}
	if w.SplitFrom != "" {
		id := TicketID(w.SplitFrom)
		t.SplitFrom = &id
	}
	return t
}

func ticketToYAML(t Ticket) ticketYAML {
	w := ticketYAML{
		ID:                    string(t.ID),
		Status:                string(t.Status),
		BlockedBy:             idsToStrings(t.BlockedBy),
		Split:                 idsToStrings(t.Split),
		Type:                  string(t.Type),
		CodeReviewFixes:       t.CodeReviewFixes,
		ExpectedContextWindow: t.ExpectedContextWindow,
		ActualContextWindow:   t.ActualContextWindow,
		ElapsedTime:           t.ElapsedTime,
		Compactions:           t.Compactions,
		Commitless:            t.Commitless,
	}
	if t.SplitFrom != nil {
		w.SplitFrom = string(*t.SplitFrom)
	}
	return w
}

func stringsToIDs(vals []string) []TicketID {
	if len(vals) == 0 {
		return nil
	}
	ids := make([]TicketID, len(vals))
	for i, v := range vals {
		ids[i] = TicketID(v)
	}
	return ids
}

func idsToStrings(ids []TicketID) []string {
	if len(ids) == 0 {
		return nil
	}
	vals := make([]string, len(ids))
	for i, id := range ids {
		vals[i] = string(id)
	}
	return vals
}

// frontmatterDelimRe recognizes a "---" delimiter line bounding a
// frontmatter block. Matched against a whole line (not "line + trailing
// content"), since a bare "---" is also valid Markdown (a horizontal rule /
// setext-heading underline) and must not be confused with prose that merely
// starts with dashes.
var frontmatterDelimRe = regexp.MustCompile(`^---\s*$`)

// splitFrontmatter looks for a "---" delimited YAML block at the very start
// of raw. hasFM is false whenever raw doesn't open with such a block, in
// which case yamlPart is empty and body is raw unchanged.
func splitFrontmatter(raw string) (yamlPart, body string, hasFM bool) {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || !frontmatterDelimRe.MatchString(lines[0]) {
		return "", raw, false
	}

	for i := 1; i < len(lines); i++ {
		if frontmatterDelimRe.MatchString(lines[i]) {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}

	return "", raw, false
}

// HasFrontmatter reports whether raw opens with a "---" delimited YAML
// frontmatter block, the same detection ParseTicketFromRaw uses to choose
// between the frontmatter and legacy bold-line parse paths. Exported for
// callers (e.g. ralphloop's status writers) that need to pick their own
// write strategy before calling ParseTicketFromRaw/MarshalTicket.
func HasFrontmatter(raw string) bool {
	_, _, hasFM := splitFrontmatter(raw)
	return hasFM
}

// MarshalTicket writes t back out as a "---" delimited YAML frontmatter
// block followed by body, unchanged, the same shape ParseTicket reads.
func MarshalTicket(t Ticket, body string) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(ticketToYAML(t))
	if err != nil {
		return nil, fmt.Errorf("marshaling ticket frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}
