package ui

import "strings"

// FuzzyMatch reports whether query is a case-insensitive subsequence of the
// fields joined by spaces, so a query may span fields (e.g. a label and its
// detail). An empty query matches everything.
func FuzzyMatch(query string, fields ...string) bool {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return true
	}
	i := 0
	for _, r := range strings.ToLower(strings.Join(fields, " ")) {
		if r == q[i] {
			i++
			if i == len(q) {
				return true
			}
		}
	}
	return false
}
