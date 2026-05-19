package log

import (
	"image/color"
	"regexp"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

type compiledRefRule struct {
	patterns []*regexp.Regexp
	color    color.Color
}

func compileRefRules(rules []config.ImportantRefRule) []compiledRefRule {
	out := make([]compiledRefRule, 0, len(rules))
	for _, rule := range rules {
		c, err := ui.ResolveColor(rule.Color)
		if err != nil {
			continue
		}
		patterns := make([]*regexp.Regexp, 0, len(rule.Patterns))
		for _, p := range rule.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			patterns = append(patterns, re)
		}
		if len(patterns) > 0 {
			out = append(out, compiledRefRule{patterns: patterns, color: c})
		}
	}
	return out
}

// matchRefRule returns the color of the first rule that matches name,
// and a boolean indicating whether a match was found.
func matchRefRule(name string, rules []compiledRefRule) (color.Color, bool) {
	for _, rule := range rules {
		for _, re := range rule.patterns {
			if re.MatchString(name) {
				return rule.color, true
			}
		}
	}
	return nil, false
}

// sortDecorations returns decorations sorted so that refs matching important
// rules appear first (grouped by rule order), followed by unmatched refs.
func sortDecorations(decorations []git.RefDecoration, rules []compiledRefRule) []git.RefDecoration {
	if len(rules) == 0 {
		return decorations
	}

	matched := make([]git.RefDecoration, 0, len(decorations))
	unmatched := make([]git.RefDecoration, 0, len(decorations))

	// For each rule (in order), collect decorations that match that rule
	// and haven't already been placed.
	placed := make(map[int]bool, len(decorations))

	for _, rule := range rules {
		for i, dec := range decorations {
			if placed[i] {
				continue
			}
			for _, re := range rule.patterns {
				if re.MatchString(dec.Name) {
					matched = append(matched, dec)
					placed[i] = true
					break
				}
			}
		}
	}

	for i, dec := range decorations {
		if !placed[i] {
			unmatched = append(unmatched, dec)
		}
	}

	return append(matched, unmatched...)
}
