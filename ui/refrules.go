package ui

import (
	"image/color"
	"regexp"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

// CompiledRefRule is a config.ImportantRefRule with its patterns pre-compiled
// and color pre-resolved, ready for repeated matching.
type CompiledRefRule struct {
	Patterns []*regexp.Regexp
	Color    color.Color
}

// CompileHideRefs compiles hide-ref patterns, silently skipping invalid ones.
func CompileHideRefs(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		out = append(out, re)
	}
	return out
}

// IsHiddenRef reports whether name matches any compiled hide-ref pattern.
func IsHiddenRef(name string, hideRefs []*regexp.Regexp) bool {
	for _, re := range hideRefs {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// CompileRefRules compiles important-ref rules, silently skipping rules with
// an invalid color or with no valid patterns.
func CompileRefRules(rules []config.ImportantRefRule) []CompiledRefRule {
	out := make([]CompiledRefRule, 0, len(rules))
	for _, rule := range rules {
		c, err := ResolveColor(rule.Color)
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
			out = append(out, CompiledRefRule{Patterns: patterns, Color: c})
		}
	}
	return out
}

// MatchRefRule returns the color of the first rule that matches name,
// and a boolean indicating whether a match was found.
func MatchRefRule(name string, rules []CompiledRefRule) (color.Color, bool) {
	for _, rule := range rules {
		for _, re := range rule.Patterns {
			if re.MatchString(name) {
				return rule.Color, true
			}
		}
	}
	return nil, false
}

// SortDecorations returns decorations sorted so that refs matching important
// rules appear first (grouped by rule order), followed by unmatched refs.
func SortDecorations(decorations []git.RefDecoration, rules []CompiledRefRule) []git.RefDecoration {
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
			for _, re := range rule.Patterns {
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
