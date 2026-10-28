package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// CodeServerStamped refuses a write to an epic field only the server stamps.
const CodeServerStamped = "server-stamped"

var epicAddressRe = regexp.MustCompile(`^([^:/\s]+):([^:/\s]+)$`)

// ParseEpicAddress splits "project:epic", the address of an epic itself (a
// ticket address has a trailing /id).
func ParseEpicAddress(s string) (project, epic string, ok bool) {
	m := epicAddressRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// EpicUpdate is the settable epic fields one `set` call changes; a nil field
// is left as it is.
type EpicUpdate struct {
	Kind                   *string
	Status                 *string
	DestinationUnconfirmed *bool
	// DesignStep is a map epic's step; a move to an earlier step refuses unless
	// AllowBackward is set.
	DesignStep    *string
	AllowBackward bool
	// BlockedBy is written as given; check it first with CheckEpicBlockedByWrite.
	BlockedBy *[]string
	// CompletedAt completes a map epic: it is stamped with the tokens and cost
	// in the same write as every other field, and an existing stamp is kept, so
	// the first completion wins.
	CompletedAt *time.Time
	// ConversationsDir is where CompletedAt reads the recorded conversations;
	// empty means conversations.DefaultDir.
	ConversationsDir string
}

// UpdateEpicFields applies u to the frontmatter of the epic ticket.md at
// ticketPath. The values are validated against the epic schema before the
// write; every other key and the body are kept.
func UpdateEpicFields(ticketPath string, u EpicUpdate) error {
	var fields EpicFields
	if u.Kind != nil {
		fields.Kind = *u.Kind
		if fields.Kind == "" {
			return fmt.Errorf("kind cannot be empty (map or loop)")
		}
	}
	if u.Status != nil {
		fields.Status = *u.Status
		if fields.Status == "" {
			return fmt.Errorf("status cannot be empty (draft or open)")
		}
	}
	if err := fields.validate(); err != nil {
		return err
	}
	if u.DesignStep != nil {
		if err := CheckDesignStep(*u.DesignStep); err != nil {
			return err
		}
	}
	var completion map[string]any
	if u.CompletedAt != nil {
		var err error
		if completion, err = mapCompletionFields(u.ConversationsDir, filepath.Dir(ticketPath), *u.CompletedAt); err != nil {
			return err
		}
	}
	return editEpicTicketMD(ticketPath, func(mapping *yaml.Node, _ *string) error {
		if u.DesignStep != nil || u.CompletedAt != nil {
			kind := effectiveKind(mapping, u.Kind)
			if u.DesignStep != nil && kind != KindMap {
				return fmt.Errorf("design_step is for a map epic; this epic is not kind: map")
			}
			if u.CompletedAt != nil && kind != KindMap {
				return fmt.Errorf("completed_at is for a map epic; this epic is not kind: map")
			}
		}
		for key, value := range completion {
			if mappingKeyIndex(mapping, key) >= 0 {
				continue
			}
			if err := setMappingValue(mapping, key, value); err != nil {
				return err
			}
		}
		if u.DesignStep != nil {
			if err := checkDesignStepMove(mapping, *u.DesignStep, u.AllowBackward); err != nil {
				return err
			}
			if err := setMappingValue(mapping, "design_step", *u.DesignStep); err != nil {
				return err
			}
		}
		if u.Kind != nil {
			if err := setMappingValue(mapping, "kind", *u.Kind); err != nil {
				return err
			}
		}
		if u.Status != nil {
			if err := setMappingValue(mapping, "status", *u.Status); err != nil {
				return err
			}
		}
		if u.DestinationUnconfirmed != nil {
			if err := setMappingValue(mapping, "destination_unconfirmed", *u.DestinationUnconfirmed); err != nil {
				return err
			}
		}
		if u.BlockedBy != nil {
			return setMappingValue(mapping, "blocked_by", *u.BlockedBy)
		}
		return nil
	})
}

// effectiveKind is the kind the epic has once this call lands: the new one if
// set, else the stored one.
func effectiveKind(mapping *yaml.Node, next *string) string {
	if next != nil {
		return *next
	}
	if i := mappingKeyIndex(mapping, "kind"); i >= 0 {
		return mapping.Content[i+1].Value
	}
	return ""
}

// checkDesignStepMove refuses a move from the stored step to an earlier one
// unless allowBackward is set.
func checkDesignStepMove(mapping *yaml.Node, next string, allowBackward bool) error {
	i := mappingKeyIndex(mapping, "design_step")
	if i < 0 || allowBackward {
		return nil
	}
	cur := mapping.Content[i+1].Value
	if slices.Index(DesignSteps, DesignStep(next)) < slices.Index(DesignSteps, DesignStep(cur)) {
		return fmt.Errorf("design_step %s is before the current %s; pass --allow-backward to move back", next, cur)
	}
	return nil
}

// SetEpicSection replaces (or appends) the "## heading" section of the epic
// ticket.md at ticketPath, keeping its frontmatter as it is.
func SetEpicSection(ticketPath, heading, content string) error {
	return editEpicTicketMD(ticketPath, func(_ *yaml.Node, body *string) error {
		*body = schema.SetSection(*body, heading, content)
		return nil
	})
}

// editEpicTicketMD runs edit on ticket.md's frontmatter mapping and body under
// the ticket lock, then writes the result atomically. The YAML node tree is
// edited in place so keys the edit does not name survive untouched.
func editEpicTicketMD(ticketPath string, edit func(mapping *yaml.Node, body *string) error) error {
	unlock, err := schema.LockTicket(ticketPath)
	if err != nil {
		return err
	}
	defer unlock()

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		return err
	}
	fm, body, ok := schema.SplitFrontmatter(string(raw))
	if !ok {
		return fmt.Errorf("%s: no frontmatter", ticketPath)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", ticketPath, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: frontmatter is not a mapping", ticketPath)
	}
	mapping := doc.Content[0]
	if err := edit(mapping, &body); err != nil {
		return err
	}
	out, err := yaml.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", ticketPath, err)
	}
	return fsutil.WriteFileAtomic(ticketPath, []byte("---\n"+string(out)+"---\n"+body))
}

// setMappingValue overwrites key's value in mapping, appending the key when
// it is absent.
func setMappingValue(mapping *yaml.Node, key string, value any) error {
	var val yaml.Node
	if err := val.Encode(value); err != nil {
		return fmt.Errorf("encoding %s: %w", key, err)
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = &val
			return nil
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &val)
	return nil
}

// EpicTicketMDPath is the ticket.md of the epic called name under projectDir,
// or CodeUnknownEpic when there is none.
func EpicTicketMDPath(projectDir, project, name string) (string, error) {
	path := filepath.Join(projectDir, name, "ticket.md")
	// The loader skips dot-dirs, so they are never epics; refusing them also
	// keeps ".." from escaping projectDir.
	unknown := &AddressError{Code: CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q in project %q", name, project)}
	if strings.HasPrefix(name, ".") {
		return "", unknown
	}
	if _, err := os.Stat(path); err != nil {
		return "", unknown
	}
	return path, nil
}
