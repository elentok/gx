package tickets

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// ImplEpicSuffix names a map's implementation epic: <map>-impl.
const ImplEpicSuffix = "-impl"

// KindLoop is the ticket.md `kind:` of an epic the scheduler runs.
const KindLoop = "loop"

// EpicFields are the epic ticket.md fields a caller may set. Everything else
// in an epic's frontmatter is server-stamped (see EpicServerStamped) or owned
// by another command, and refused when it arrives in a body.
type EpicFields struct {
	Kind                   string `yaml:"kind,omitempty"`
	Status                 string `yaml:"status,omitempty"`
	ProvisionalDestination bool   `yaml:"provisional_destination,omitempty"`

	// CreatedAt is stamped by CreateEpic. A caller-supplied value refuses.
	CreatedAt time.Time `yaml:"created_at,omitempty"`
}

// EpicServerStamped are the epic fields only the server writes.
// The land_block_* keys are stored epic frontmatter the server still writes.
var EpicServerStamped = []string{"created_at", "completed_at", "merged_at", "land_block_kind", "land_block_reason", "evolved_at", "evolve_skipped", "evolve_ticket"}

const epicSettableList = "kind, status, provisional_destination"

// ParseEpicFields decodes a frontmatter block holding only settable epic
// fields. A server-stamped or unknown key refuses, naming which it is. The
// values are checked too (see validate).
func ParseEpicFields(yamlPart string) (EpicFields, error) {
	var f EpicFields
	var keys map[string]any
	if err := yaml.Unmarshal([]byte(yamlPart), &keys); err != nil {
		return f, fmt.Errorf("epic frontmatter: %w", err)
	}
	for key := range keys {
		switch key {
		case "kind", "status", "provisional_destination":
		default:
			if slices.Contains(EpicServerStamped, key) {
				return f, fmt.Errorf("epic frontmatter: %s is server-stamped and cannot be set (settable: %s)", key, epicSettableList)
			}
			return f, fmt.Errorf("epic frontmatter: unknown field %q (settable: %s)", key, epicSettableList)
		}
	}
	if err := yaml.Unmarshal([]byte(yamlPart), &f); err != nil {
		return f, fmt.Errorf("epic frontmatter: %w", err)
	}
	if err := f.validate(); err != nil {
		return f, fmt.Errorf("epic frontmatter: %w", err)
	}
	return f, nil
}

// validate checks the values: kind is map or loop, status is draft or open
// (either may be empty, meaning not given).
func (f EpicFields) validate() error {
	if !f.CreatedAt.IsZero() {
		return errors.New("created_at is server-stamped and cannot be set")
	}
	switch f.Kind {
	case "", KindMap, KindLoop:
	default:
		return fmt.Errorf("kind %q; want map or loop", f.Kind)
	}
	switch schema.Status(f.Status) {
	case "", schema.StatusOpen, schema.StatusDraft:
	default:
		return fmt.Errorf("status %q; a new epic is draft or open", f.Status)
	}
	return nil
}

// CreateEpic writes a new epic's ticket.md under projectDir. A body may open
// with its own frontmatter block of settable fields; kind and status given
// there and in fields must agree. Every check runs before the first write, so
// a refusal leaves the store untouched. An epic directory that already exists
// refuses, with or without a ticket.md.
func CreateEpic(projectDir, name string, fields EpicFields, body string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/:\\") {
		return "", fmt.Errorf("invalid epic name %q", name)
	}
	if err := fields.validate(); err != nil {
		return "", err
	}
	if yamlPart, text, ok := schema.SplitFrontmatter(strings.TrimLeft(body, "\n")); ok {
		fromBody, err := ParseEpicFields(yamlPart)
		if err != nil {
			return "", err
		}
		if fields, err = mergeEpicFields(fields, fromBody); err != nil {
			return "", err
		}
		body = text
	}
	if fields.Kind == "" {
		return "", errors.New("kind is required (map or loop)")
	}
	if fields.Status == "" {
		fields.Status = string(schema.StatusDraft)
	}

	epicPath := filepath.Join(projectDir, name)
	if _, err := os.Stat(epicPath); err == nil {
		return "", fmt.Errorf("epic %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	fields.CreatedAt = time.Now().UTC()
	fm, err := yaml.Marshal(fields)
	if err != nil {
		return "", err
	}
	// Stamp the map before creating anything, so a failure leaves no new epic.
	if mapPath, ok := mapCompletedBy(projectDir, name, fields.Kind); ok {
		if err := StampEpicFields(mapPath, map[string]any{"completed_at": fields.CreatedAt}); err != nil {
			return "", err
		}
	}
	text := strings.Trim(body, "\n")
	if text == "" {
		text = "# " + name
	}
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(epicPath, "ticket.md")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, err = io.WriteString(f, "---\n"+string(fm)+"---\n\n"+text+"\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return epicPath, err
	}
	return epicPath, nil
}

// implEpicPath is where a map's <map>-impl epic lives.
func implEpicPath(root, mapName string) string {
	return filepath.Join(root, mapName+ImplEpicSuffix)
}

// mapCompletedBy returns the map that creating the loop epic name completes.
// Creating <map>-impl is what completes the map; BackfillEpicTimes applies the
// same rule to epics that already exist.
func mapCompletedBy(root, name, kind string) (string, bool) {
	mapName, ok := strings.CutSuffix(name, ImplEpicSuffix)
	if !ok || kind != KindLoop {
		return "", false
	}
	mapPath := filepath.Join(root, mapName)
	return mapPath, IsMapEpic(mapPath)
}

// mergeEpicFields combines the flag-supplied and body-supplied fields; a
// field set on both sides must agree.
func mergeEpicFields(flags, body EpicFields) (EpicFields, error) {
	for _, p := range []struct {
		name string
		dst  *string
		src  string
	}{{"kind", &flags.Kind, body.Kind}, {"status", &flags.Status, body.Status}} {
		switch {
		case p.src == "":
		case *p.dst == "":
			*p.dst = p.src
		case *p.dst != p.src:
			return flags, fmt.Errorf("--%s %s conflicts with the body's %s: %s", p.name, *p.dst, p.name, p.src)
		}
	}
	flags.ProvisionalDestination = flags.ProvisionalDestination || body.ProvisionalDestination
	return flags, nil
}
