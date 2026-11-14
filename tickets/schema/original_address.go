package schema

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// OriginalAddresses is the original_address field: the address the ticket was
// created under or, for a promoted ticket, the follow-up address(es) it came
// from. On disk a single entry is a YAML string and several are a list, so
// files from before the list form still parse and round-trip unchanged.
type OriginalAddresses []string

// Has reports whether addr is one of the entries.
func (o OriginalAddresses) Has(addr string) bool { return slices.Contains(o, addr) }

// First returns the first entry, or "" for an empty list.
func (o OriginalAddresses) First() string {
	if len(o) == 0 {
		return ""
	}
	return o[0]
}

// String joins the entries with ", "; a single entry is returned as is.
func (o OriginalAddresses) String() string { return strings.Join(o, ", ") }

// MarshalYAML writes a single entry as a string and several as a list.
func (o OriginalAddresses) MarshalYAML() (any, error) {
	if len(o) == 1 {
		return o[0], nil
	}
	return []string(o), nil
}

// UnmarshalYAML accepts a string or a list of strings.
func (o *OriginalAddresses) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var s string
		if err := n.Decode(&s); err != nil {
			return err
		}
		if s == "" {
			*o = nil
		} else {
			*o = OriginalAddresses{s}
		}
		return nil
	case yaml.SequenceNode:
		var l []string
		if err := n.Decode(&l); err != nil {
			return err
		}
		*o = l
		return nil
	}
	return fmt.Errorf("line %d: original_address must be a string or a list of strings", n.Line)
}

// ResolutionHasLine reports whether the body's Resolution section holds line,
// ignoring surrounding whitespace.
func ResolutionHasLine(body, line string) bool {
	for l := range strings.SplitSeq(Section(body, "Resolution"), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
