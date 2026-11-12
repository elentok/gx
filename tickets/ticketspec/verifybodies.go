package ticketspec

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// VerifyTicket is a packed entry with its body.
type VerifyTicket struct {
	VerifyEntry
	Body string
}

// BodyFileName is the body file written next to the manifest.
func (t VerifyTicket) BodyFileName() string { return "verify-body-" + t.Slug + ".md" }

// BuildVerifyTickets packs the plan and writes each ticket's body from the
// spec. Verification entries, story texts and Mock states lines are copied word
// for word with `{{` escaped. Every problem is reported.
func BuildVerifyTickets(plan VerifyPlan, spec, specPath string) ([]VerifyTicket, error) {
	stories, _ := SpecSection(spec, "User Stories")
	verification, ok := SpecSection(spec, "Verification")
	if !ok {
		return nil, fmt.Errorf("%s has no ## Verification section", specPath)
	}
	outOfScope, _ := SpecSection(spec, "Out of Scope")

	var errs []error
	byNumber := map[int]PlanStory{}
	for _, s := range plan.Stories {
		byNumber[s.Story] = s
	}
	lookup := func(s PlanStory) (text, entry string) {
		var ok bool
		if text, ok = SpecListItem(stories, s.Story); !ok {
			errs = append(errs, fmt.Errorf("story %d has no entry in ## User Stories of %s", s.Story, specPath))
		}
		if entry, ok = SpecListItem(verification, s.Story); !ok {
			errs = append(errs, fmt.Errorf("story %d has no entry in ## Verification of %s", s.Story, specPath))
		}
		return text, entry
	}

	scenario := plan.Scenario
	if scenario == "" {
		scenario = "none"
	}
	entries := PackVerifyPlan(plan)
	lone := len(entries) == 1
	var out []VerifyTicket
	for _, e := range entries {
		var b strings.Builder
		b.WriteString("Round: 1\n")
		switch e.Slug {
		case "verify-collect":
			b.WriteString("Role: collect\n")
			fmt.Fprintf(&b, "\nSpec: %s\n", specPath)
		default:
			if !lone {
				b.WriteString("Role: part\n")
			}
			fmt.Fprintf(&b, "\nSpec: %s\nFixture scenario: %s\n", specPath, scenario)
			if e.Note != "" {
				fmt.Fprintf(&b, "\nNote: %s\n", e.Note)
			}
			var states []string
			for _, n := range e.Stories {
				s := byNumber[n]
				text, entry := lookup(s)
				fmt.Fprintf(&b, "\n## Story %d (estimate %s)\n\nStory:\n\n%s\n\nVerification:\n\n%s\n",
					n, tokensK(storyTokens(s)), fence(text), fence(entry))
				for _, m := range s.MockStates {
					if !slices.Contains(states, m) {
						states = append(states, m)
					}
				}
			}
			if len(states) > 0 {
				b.WriteString("\n## Mock states\n")
				for _, m := range states {
					line, ok := SpecMockLine(verification, m)
					if !ok {
						errs = append(errs, fmt.Errorf("mock state %q has no line in the Mock states list of %s", m, specPath))
					}
					fmt.Fprintf(&b, "\n(estimate %s)\n\n%s\n", tokensK(MockStateTokens), fence(line))
				}
			}
		}
		if (lone || e.Slug == "verify-collect") && outOfScope != "" {
			fmt.Fprintf(&b, "\n## Out of Scope\n\n%s\n", EscapePlaceholders(outOfScope))
		}
		out = append(out, VerifyTicket{VerifyEntry: e, Body: b.String()})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// isVerifySlug reports whether slug is one of the rows verify-plan owns.
func isVerifySlug(slug string) bool {
	return slug == "verify" || slug == "verify-collect" || strings.HasPrefix(slug, "verify-part-")
}

// dropRows removes the rows of manifest that drop selects (it gets the row's
// slug and body) and returns the body of each removed row.
func dropRows(manifest []byte, drop func(slug, body string) bool) (out []byte, bodies []string, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(manifest, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return manifest, nil, nil
	}
	seq := doc.Content[0]
	seq.Content = slices.DeleteFunc(seq.Content, func(item *yaml.Node) bool {
		if item.Kind != yaml.MappingNode {
			return false
		}
		var slug, body string
		for i := 0; i+1 < len(item.Content); i += 2 {
			switch item.Content[i].Value {
			case "slug":
				slug = item.Content[i+1].Value
			case "body":
				body = item.Content[i+1].Value
			}
		}
		if !drop(slug, body) {
			return false
		}
		bodies = append(bodies, body)
		return true
	})
	if len(seq.Content) == 0 {
		return nil, bodies, nil
	}
	out, err = yaml.Marshal(&doc)
	return out, bodies, err
}

// appendRows adds rows at the end of manifest (possibly empty).
func appendRows(manifest []byte, rows []ManifestRow) ([]byte, error) {
	add, err := yaml.Marshal(rows)
	if err != nil {
		return nil, err
	}
	if len(manifest) > 0 && manifest[len(manifest)-1] != '\n' {
		manifest = append(manifest, '\n')
	}
	return append(manifest, add...), nil
}

// AppendManifest returns manifest (possibly empty) with one entry per ticket
// added at the end. Existing verify rows are dropped first, so a re-run
// replaces them instead of duplicating them.
func AppendManifest(manifest []byte, ts []VerifyTicket) ([]byte, error) {
	rows := make([]ManifestRow, 0, len(ts))
	for _, t := range ts {
		rows = append(rows, ManifestRow{t.Slug, t.Type, t.BlockedBy, t.ExpectedContextWindow, t.BodyFileName()})
	}
	out, _, err := dropRows(manifest, func(slug, _ string) bool { return isVerifySlug(slug) })
	if err != nil {
		return nil, err
	}
	return appendRows(out, rows)
}

// fence puts text in a code fence long enough to hold any backticks in it, so
// the text inside stays word for word. `{{` is escaped for publish.
func fence(text string) string {
	ticks := "```"
	for strings.Contains(text, ticks) {
		ticks += "`"
	}
	return ticks + "text\n" + EscapePlaceholders(text) + "\n" + ticks
}

func tokensK(n int) string { return fmt.Sprintf("%dK", n/1000) }
