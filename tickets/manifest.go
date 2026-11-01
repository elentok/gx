package tickets

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/elentok/gx/tickets/schema"
)

// CommitSlug is reserved: {{commit}} is the commit stamp, not a cross-reference.
const CommitSlug = "commit"

var (
	manifestSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	digitsOnlyRe   = regexp.MustCompile(`^\d+$`)
	// placeholderRe matches the escape `\{{` first, so an escaped placeholder is
	// never read as a real one.
	placeholderRe = regexp.MustCompile(`\\\{\{|\{\{([a-z][a-z0-9-]*)\}\}`)
)

// ManifestEntry is one ticket of a publish manifest.
type ManifestEntry struct {
	Slug                  string   `yaml:"slug"`
	Type                  string   `yaml:"type"`
	BlockedBy             []string `yaml:"blocked_by"`
	ExpectedContextWindow int      `yaml:"expected_context_window"`
	BodyFile              string   `yaml:"body"`

	// Body is the raw body read from BodyFile.
	Body string `yaml:"-"`
}

// Manifest is a checked publish manifest. Entries are in ID-assignment order:
// manifest order, with each blocker moved up before its dependents.
type Manifest struct {
	Entries []ManifestEntry
}

// ParseManifest reads and checks the manifest at path. epicIDs are the IDs of
// the tickets already in the target epic, which bare-ID blockers must name.
// Every problem is reported; on any problem the manifest is nil.
func ParseManifest(path string, epicIDs []string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []ManifestEntry
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", path, err)
	}
	return buildManifest(entries, filepath.Dir(path), epicIDs)
}

func buildManifest(entries []ManifestEntry, dir string, epicIDs []string) (*Manifest, error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	slugs := map[string]bool{}
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Slug == CommitSlug:
			fail("slug %q is reserved: {{commit}} is the commit stamp", e.Slug)
		case digitsOnlyRe.MatchString(e.Slug):
			fail("slug %q is digits only; a slug must start with a letter", e.Slug)
		case !manifestSlugRe.MatchString(e.Slug):
			fail("slug %q must match [a-z][a-z0-9-]*", e.Slug)
		case slugs[e.Slug]:
			fail("duplicate slug %q", e.Slug)
		}
		slugs[e.Slug] = true

		if !schema.TicketType(e.Type).Valid() {
			fail("%s: unknown type %q", e.Slug, e.Type)
		}
		if w := e.ExpectedContextWindow; w < 0 || (w != 0 && w < schema.MinExpectedContextWindow) {
			fail("%s: expected_context_window %d must be 0 or at least %d", e.Slug, w, schema.MinExpectedContextWindow)
		}
		if e.BodyFile == "" {
			fail("%s: body is required", e.Slug)
		} else if b, err := os.ReadFile(filepath.Join(dir, e.BodyFile)); err != nil {
			fail("%s: body file %q: %v", e.Slug, e.BodyFile, errors.Unwrap(err))
		} else {
			e.Body = string(b)
		}
	}

	for _, e := range entries {
		for _, m := range placeholderRe.FindAllStringSubmatch(e.Body, -1) {
			if s := m[1]; s != "" && s != CommitSlug && !slugs[s] {
				fail("%s: body references unknown slug {{%s}}", e.Slug, s)
			}
		}
		for _, ref := range e.BlockedBy {
			switch {
			case strings.Contains(ref, "/"):
				fail("%s: blocked_by %q names another epic; only this epic's tickets are allowed", e.Slug, ref)
			case digitsOnlyRe.MatchString(ref):
				if !slices.Contains(epicIDs, ref) {
					fail("%s: blocked_by ID %q is not a ticket of the target epic", e.Slug, ref)
				}
			case !slugs[ref]:
				fail("%s: blocked_by %q is not a slug of this manifest", e.Slug, ref)
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	ordered, err := orderEntries(entries)
	if err != nil {
		return nil, err
	}
	return &Manifest{Entries: ordered}, nil
}

// orderEntries is a stable topological sort: entries keep manifest order and
// each blocker is pulled up in front of its first dependent.
func orderEntries(entries []ManifestEntry) ([]ManifestEntry, error) {
	bySlug := map[string]ManifestEntry{}
	for _, e := range entries {
		bySlug[e.Slug] = e
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var out []ManifestEntry
	var stack []string
	var visit func(slug string) error
	visit = func(slug string) error {
		switch state[slug] {
		case done:
			return nil
		case visiting:
			i := slices.Index(stack, slug)
			cycle := append(slices.Clone(stack[i:]), slug)
			return fmt.Errorf("dependency cycle: %s", strings.Join(cycle, " -> "))
		}
		state[slug] = visiting
		stack = append(stack, slug)
		for _, ref := range bySlug[slug].BlockedBy {
			if _, ok := bySlug[ref]; !ok {
				continue // an existing ticket ID
			}
			if err := visit(ref); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[slug] = done
		out = append(out, bySlug[slug])
		return nil
	}
	for _, e := range entries {
		if err := visit(e.Slug); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RenderManifestBody returns body with each {{slug}} replaced by ids[slug] and `\{{`
// turned into a literal `{{`. {{commit}} and other brace forms pass through.
func RenderManifestBody(body string, ids map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(body, func(m string) string {
		if m == `\{{` {
			return "{{"
		}
		slug := m[2 : len(m)-2]
		if id, ok := ids[slug]; ok {
			return id
		}
		return m
	})
}
