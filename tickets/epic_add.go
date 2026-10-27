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

	"github.com/elentok/gx/conversations"
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
	DestinationUnconfirmed bool   `yaml:"destination_unconfirmed,omitempty"`

	// CreatedAt is stamped by CreateEpic. A caller-supplied value refuses.
	CreatedAt time.Time `yaml:"created_at,omitempty"`
}

// EpicServerStamped are the epic fields only the server writes.
// The land_block_* keys are stored epic frontmatter the server still writes.
var EpicServerStamped = []string{"created_at", "completed_at", "merged_at", "queued_at", "land_block_kind", "land_block_reason", "land_block_files", "trunk_fast_forwarded_at", "trunk_conflict_backup", "trunk_conflict_files", "evolved_at", "evolve_skipped", "evolve_ticket", "lines_added", "lines_removed", "total_tokens", "actual_cost", "had_codex", "friction"}

const epicSettableList = "kind, status, destination_unconfirmed"

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
		case "kind", "status", "destination_unconfirmed":
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
		if err := StampMapCompletion(mapPath, fields.CreatedAt); err != nil {
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

// StampMapCompletion stamps the map at mapPath as completed at at: its
// completed_at, plus the tokens and cost of the conversations recorded against
// it or its tickets. A map created before gx recorded conversations, or with a
// conversation lacking totals, gets no tokens or cost. Stamping never
// overwrites, so the first completion wins.
func StampMapCompletion(mapPath string, at time.Time) error {
	fields, err := mapCompletionFields(mapPath, at)
	if err != nil {
		return err
	}
	return StampEpicFields(mapPath, fields)
}

// mapCompletionFields are the frontmatter keys StampMapCompletion writes.
func mapCompletionFields(mapPath string, at time.Time) (map[string]any, error) {
	fields := map[string]any{"completed_at": at}
	root, err := conversations.DefaultDir()
	if err != nil {
		return nil, err
	}
	label := ProjectName(filepath.Dir(mapPath)) + ":" + filepath.Base(mapPath)
	sum, ok, err := conversations.SumEpicTotals(root, label, LoadEpicTicketMD(mapPath).CreatedAt)
	if err != nil {
		return nil, err
	}
	if ok {
		fields["total_tokens"] = int64(sum.TotalTokens)
		fields["actual_cost"] = sum.Cost
	}
	return fields, nil
}

// implEpicPath is where a map's <map>-impl epic lives.
func implEpicPath(root, mapName string) string {
	return filepath.Join(root, mapName+ImplEpicSuffix)
}

// EpicSpecPath names the spec of the epic at epicPath, or "" when there is
// none. A repo spec (docs/specs/<epic>.md, or <map>.md for a <map>-impl epic)
// is named repo-relative, the epic's own spec.md by its store path.
func EpicSpecPath(epicPath string) string {
	epic := filepath.Base(epicPath)
	projectDir := filepath.Dir(epicPath)
	store := filepath.Dir(projectDir)

	if checkout := TrunkCheckout(store, filepath.Base(projectDir)); checkout != "" {
		slugs := []string{epic}
		if name, ok := strings.CutSuffix(epic, ImplEpicSuffix); ok {
			slugs = append(slugs, name)
		}
		for _, slug := range slugs {
			rel := filepath.Join("docs", "specs", slug+".md")
			if fileExists(filepath.Join(checkout, rel)) {
				return rel
			}
		}
	}
	if own := filepath.Join(epicPath, "spec.md"); fileExists(own) {
		return own
	}
	return ""
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
	flags.DestinationUnconfirmed = flags.DestinationUnconfirmed || body.DestinationUnconfirmed
	return flags, nil
}
