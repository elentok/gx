package tickets

import (
	"bytes"
	"fmt"
	"io"
	"slices"

	"gopkg.in/yaml.v3"
)

// VerifyPlan is the verify plan the agent writes: one entry per story with
// user-visible behaviour.
type VerifyPlan struct {
	Scenario string      `yaml:"scenario"` // fixture scenario to start; empty means none
	Stories  []PlanStory `yaml:"stories"`
}

// PlanStory is one story's verify items. MockStates are names, so a state
// shared by two stories in a part is counted once.
type PlanStory struct {
	Story       int      `yaml:"story"`
	Group       string   `yaml:"group"` // scenario group; empty means none
	WebFlows    int      `yaml:"web_flows"`
	CLIChecks   int      `yaml:"cli_checks"`
	GoTests     int      `yaml:"go_tests"`
	SkillQuotes int      `yaml:"skill_quotes"`
	MockStates  []string `yaml:"mock_states"`
}

// VerifyEntry is one planned verify ticket, before its body is written.
type VerifyEntry struct {
	Slug                  string
	Type                  string
	ExpectedContextWindow int
	BlockedBy             []string
	Stories               []int
	Note                  string // set when a lone story is over the limit
}

// ParseVerifyPlan reads a plan and rejects unknown fields, empty plans,
// missing or repeated story numbers and negative counts.
func ParseVerifyPlan(data []byte) (VerifyPlan, error) {
	var p VerifyPlan
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && err != io.EOF {
		return VerifyPlan{}, fmt.Errorf("parse plan: %w", err)
	}
	if len(p.Stories) == 0 {
		return VerifyPlan{}, fmt.Errorf("plan has no stories")
	}
	seen := map[int]bool{}
	for _, s := range p.Stories {
		switch {
		case s.Story <= 0:
			return VerifyPlan{}, fmt.Errorf("plan story needs a positive number")
		case seen[s.Story]:
			return VerifyPlan{}, fmt.Errorf("story %d appears twice", s.Story)
		case min(s.WebFlows, s.CLIChecks, s.GoTests, s.SkillQuotes) < 0:
			return VerifyPlan{}, fmt.Errorf("story %d has a negative count", s.Story)
		}
		seen[s.Story] = true
	}
	return p, nil
}

// storyTokens is a story's estimate without its mock states, which are counted
// once per distinct state.
func storyTokens(s PlanStory) int {
	return s.WebFlows*WebFlowTokens + s.CLIChecks*CLICheckTokens +
		s.GoTests*GoTestRunTokens + s.SkillQuotes*SkillQuoteTokens
}

// partTokens is a part's expected tokens: the base, every story's items, and
// each distinct mock state once.
func partTokens(stories []PlanStory) int {
	total := VerifyPartBaseTokens
	states := map[string]bool{}
	for _, s := range stories {
		total += storyTokens(s)
		for _, m := range s.MockStates {
			states[m] = true
		}
	}
	return total + len(states)*MockStateTokens
}

// PackVerifyPlan packs stories greedily, in story order, into parts at or
// under SplitLimitTokens. Stories sharing a group stay together unless the
// group alone is over the limit; a lone story over the limit gets its own part
// with a note. One part gives a lone `verify` entry; more give `verify-part-N`
// entries plus a `verify-collect` blocked by all of them.
func PackVerifyPlan(plan VerifyPlan) []VerifyEntry {
	var parts [][]PlanStory
	var cur []PlanStory
	flush := func() {
		if len(cur) > 0 {
			parts = append(parts, cur)
			cur = nil
		}
	}
	place := func(unit []PlanStory) {
		if len(cur) > 0 && partTokens(append(slices.Clone(cur), unit...)) > SplitLimitTokens {
			flush()
		}
		cur = append(cur, unit...)
	}
	for _, unit := range groupUnits(plan.Stories) {
		if len(unit) > 1 && partTokens(unit) > SplitLimitTokens {
			for _, s := range unit {
				place([]PlanStory{s})
			}
			continue
		}
		place(unit)
	}
	flush()

	var out []VerifyEntry
	for i, p := range parts {
		e := VerifyEntry{Slug: "verify", Type: "verify", ExpectedContextWindow: partTokens(p)}
		for _, s := range p {
			e.Stories = append(e.Stories, s.Story)
		}
		if len(parts) > 1 {
			e.Slug = fmt.Sprintf("verify-part-%d", i+1)
		}
		if len(p) == 1 && e.ExpectedContextWindow > SplitLimitTokens {
			e.Note = fmt.Sprintf("over budget: story %d alone needs %d tokens, limit %d",
				p[0].Story, e.ExpectedContextWindow, SplitLimitTokens)
		}
		out = append(out, e)
	}
	if len(out) > 1 {
		collect := VerifyEntry{Slug: "verify-collect", Type: "verify", ExpectedContextWindow: VerifyCollectTokens}
		for _, e := range out {
			collect.BlockedBy = append(collect.BlockedBy, e.Slug)
		}
		out = append(out, collect)
	}
	return out
}

// groupUnits turns the stories into packing units in order of first
// appearance: a scenario group is one unit, an ungrouped story its own.
func groupUnits(stories []PlanStory) [][]PlanStory {
	var units [][]PlanStory
	index := map[string]int{}
	for _, s := range stories {
		if s.Group == "" {
			units = append(units, []PlanStory{s})
			continue
		}
		if i, ok := index[s.Group]; ok {
			units[i] = append(units[i], s)
			continue
		}
		index[s.Group] = len(units)
		units = append(units, []PlanStory{s})
	}
	return units
}

// WriteEstimateTable prints every estimate verify-plan uses.
func WriteEstimateTable(w io.Writer) {
	rows := []struct {
		name   string
		tokens int
	}{
		{"web flow", WebFlowTokens},
		{"CLI check", CLICheckTokens},
		{"Go test run", GoTestRunTokens},
		{"skill quote", SkillQuoteTokens},
		{"mock state", MockStateTokens},
		{"part base", VerifyPartBaseTokens},
		{"collect estimate", VerifyCollectTokens},
		{"split limit", SplitLimitTokens},
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-18s %dK\n", r.name, r.tokens/1000)
	}
}
