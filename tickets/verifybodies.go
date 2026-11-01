package tickets

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

// AppendManifest returns manifest (possibly empty) with one entry per ticket
// added at the end.
func AppendManifest(manifest []byte, ts []VerifyTicket) ([]byte, error) {
	type row struct {
		Slug      string   `yaml:"slug"`
		Type      string   `yaml:"type"`
		BlockedBy []string `yaml:"blocked_by,omitempty"`
		Window    int      `yaml:"expected_context_window"`
		Body      string   `yaml:"body"`
	}
	rows := make([]row, 0, len(ts))
	for _, t := range ts {
		rows = append(rows, row{t.Slug, t.Type, t.BlockedBy, t.ExpectedContextWindow, t.BodyFileName()})
	}
	add, err := yaml.Marshal(rows)
	if err != nil {
		return nil, err
	}
	out := manifest
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, add...), nil
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
