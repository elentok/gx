package ticketspec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/elentok/gx/tickets/schema"
)

// DraftMarker stands in for the two agent-written parts of a drafted body.
// lint-claims and publish refuse a body that still has one.
const DraftMarker = "<!-- TODO -->"

// DraftBodyPrefix starts the body file name of every ticket `draft` writes, so
// a re-run can tell its own manifest rows from the rows of other commands.
const DraftBodyPrefix = "draft-body-"

// HasDraftMarker reports whether body still holds an unwritten agent part.
func HasDraftMarker(body string) bool { return strings.Contains(body, DraftMarker) }

// DraftPlan is the input of `gx tickets draft`.
type DraftPlan struct {
	// Scenario is the fixture scenario named in every Mock check section.
	Scenario string        `yaml:"scenario"`
	Tickets  []DraftTicket `yaml:"tickets"`
}

// DraftTicket is one ticket of a draft plan. MockStates are lines such as
// `$(gx tickets root)/demo/mocks/01.html, calm, 1100px, parts: top-bar`.
type DraftTicket struct {
	Slug           string   `yaml:"slug"`
	Title          string   `yaml:"title"`
	Type           string   `yaml:"type"`
	BlockedBy      []string `yaml:"blocked_by"`
	Window         int      `yaml:"window"`
	EntrySymbols   []string `yaml:"entry_symbols"`
	TestFiles      []string `yaml:"test_files"`
	MockStates     []string `yaml:"mock_states"`
	ChangedSymbols []string `yaml:"changed_symbols"`
	ChangedFields  []string `yaml:"changed_fields"`
	// MakeTargets must exist in the Makefile; Helpers are Go symbols whose
	// declaration and comment are printed; Migrations asks for the highest
	// migration number; Playwright checks one -g pattern per flow file.
	MakeTargets []string          `yaml:"make_targets"`
	Helpers     []string          `yaml:"helpers"`
	Migrations  bool              `yaml:"migrations"`
	Playwright  []PlaywrightCheck `yaml:"playwright"`

	// Callers maps an entry or changed symbol to its caller list, already
	// labelled; `draft` fills it in, the plan file does not.
	Callers map[string]string `yaml:"-"`
	// SeamFacts are printed lines under Test seams. Consumers and FieldTests
	// map a changed field to its non-test and test mentions.
	SeamFacts  []string          `yaml:"-"`
	Consumers  map[string]string `yaml:"-"`
	FieldTests map[string]string `yaml:"-"`
}

// PlaywrightCheck names a flow file, the flow numbers to run and the -g pattern.
type PlaywrightCheck struct {
	File  string `yaml:"file"`
	Flows []int  `yaml:"flows"`
	Grep  string `yaml:"grep"`
}

// BodyFileName is the body file written next to the manifest.
func (t DraftTicket) BodyFileName() string { return DraftBodyPrefix + t.Slug + ".md" }

// ParseDraftPlan reads a draft plan and reports every problem.
func ParseDraftPlan(data []byte) (DraftPlan, error) {
	var p DraftPlan
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("draft plan: %w", err)
	}
	var errs []error
	if len(p.Tickets) == 0 {
		errs = append(errs, errors.New("draft plan has no tickets"))
	}
	seen := map[string]bool{}
	for i := range p.Tickets {
		t := &p.Tickets[i]
		if t.Type == "" {
			t.Type = string(schema.TypeImplement)
		}
		switch {
		case !manifestSlugRe.MatchString(t.Slug):
			errs = append(errs, fmt.Errorf("ticket %d: slug %q must match [a-z][a-z0-9-]*", i+1, t.Slug))
		case seen[t.Slug]:
			errs = append(errs, fmt.Errorf("duplicate slug %q", t.Slug))
		}
		seen[t.Slug] = true
		if strings.TrimSpace(t.Title) == "" {
			errs = append(errs, fmt.Errorf("%s: title is required", t.Slug))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return p, err
	}
	return p, nil
}

// RenderDraftBody writes the body of t. The command owns the header, Test
// seams, Where to look and Mock check; whatToBuild and acceptance are the agent
// parts, and an empty one becomes the marker. Plan text has `{{` escaped.
func RenderDraftBody(t DraftTicket, scenario, whatToBuild, acceptance string) string {
	agent := func(s string) string {
		if s == "" {
			return DraftMarker
		}
		return s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## What to build\n\n%s\n", EscapePlaceholders(t.Title), agent(whatToBuild))

	b.WriteString("\n## Test seams\n\n")
	if len(t.TestFiles) == 0 {
		b.WriteString("none — the plan names no test files.\n")
	}
	list := func(items []string) {
		for _, it := range items {
			fmt.Fprintf(&b, "- `%s`\n", EscapePlaceholders(it))
		}
	}
	list(t.TestFiles)
	for _, f := range t.SeamFacts {
		fmt.Fprintf(&b, "- %s\n", EscapePlaceholders(f))
	}

	b.WriteString("\n## Where to look\n\n")
	if len(t.EntrySymbols)+len(t.ChangedSymbols)+len(t.ChangedFields) == 0 {
		b.WriteString("none — the plan names no entry symbols.\n")
	}
	for _, g := range []struct {
		label string
		items []string
	}{{"Entry points", t.EntrySymbols}, {"Changed symbols", t.ChangedSymbols}, {"Changed fields", t.ChangedFields}} {
		if len(g.items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s:\n", g.label)
		for _, it := range g.items {
			fmt.Fprintf(&b, "  - `%s`\n", EscapePlaceholders(it))
			if c, ok := t.Callers[it]; ok {
				fmt.Fprintf(&b, "    - callers (%s)\n", EscapePlaceholders(c))
			}
			if c, ok := t.Consumers[it]; ok {
				fmt.Fprintf(&b, "    - consumers (%s)\n", EscapePlaceholders(c))
				fmt.Fprintf(&b, "    - existing tests mentioning it (%s)\n", EscapePlaceholders(t.FieldTests[it]))
			}
		}
	}

	if len(t.MockStates) > 0 {
		b.WriteString("\n## Mock check\n\n")
		if scenario != "" {
			fmt.Fprintf(&b, "Fixture scenario: %s\n\n", EscapePlaceholders(scenario))
		}
		for _, m := range t.MockStates {
			fmt.Fprintf(&b, "- %s\n", EscapePlaceholders(m))
		}
	}

	fmt.Fprintf(&b, "\n## Acceptance criteria\n\n%s\n", agent(acceptance))
	return b.String()
}

// AgentParts returns the agent-written parts of an earlier body, with the
// marker counted as unwritten.
func AgentParts(body string) (whatToBuild, acceptance string) {
	keep := func(s string) string {
		if s == DraftMarker {
			return ""
		}
		return s
	}
	return keep(schema.Section(body, "What to build")), keep(schema.Section(body, "Acceptance criteria"))
}

// ManifestRow is one manifest entry that a command writes.
type ManifestRow struct {
	Slug      string   `yaml:"slug"`
	Type      string   `yaml:"type"`
	BlockedBy []string `yaml:"blocked_by,omitempty"`
	Window    int      `yaml:"expected_context_window"`
	Body      string   `yaml:"body"`
}

// AppendDraftManifest returns manifest with the draft rows of an earlier run
// replaced by rows, plus the body files of the dropped rows that rows no longer
// name. Rows of other commands stay.
func AppendDraftManifest(manifest []byte, rows []ManifestRow) (out []byte, removedBodies []string, err error) {
	isDraft := func(body string) bool { return strings.HasPrefix(body, DraftBodyPrefix) }
	out, dropped, err := dropRows(manifest, func(_, body string) bool { return isDraft(body) })
	if err != nil {
		return nil, nil, err
	}
	for _, d := range dropped {
		keep := false
		for _, r := range rows {
			keep = keep || r.Body == d
		}
		if !keep {
			removedBodies = append(removedBodies, d)
		}
	}
	out, err = appendRows(out, rows)
	return out, removedBodies, err
}
