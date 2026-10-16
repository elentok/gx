package agents

import (
	"slices"
	"strings"
	"unicode"

	"github.com/elentok/gx/ui"
)

// Fuzzy score weights: every matched rune scores, more so at a word start or
// right after the previous match; each skipped rune costs, up to a cap per gap.
const (
	scoreMatch       = 1
	scoreBoundary    = 8
	scoreConsecutive = 6
	maxGapPenalty    = 8
)

// fuzzyScore matches each space-separated term of query against text as an
// in-order subsequence, ignoring case. ok is false when a term does not
// match; pos is the matched rune indices of text, ascending. An empty query
// matches everything with score 0.
func fuzzyScore(query, text string) (score int, pos []int, ok bool) {
	runes := lowerRunes(text)
	for _, term := range strings.Fields(query) {
		s, p, found := bestTermMatch(lowerRunes(term), runes)
		if !found {
			return 0, nil, false
		}
		score += s
		pos = append(pos, p...)
	}
	slices.Sort(pos)
	return score, slices.Compact(pos), true
}

// lowerRunes lowers s rune by rune, so indices stay those of []rune(s).
func lowerRunes(s string) []rune {
	runes := []rune(s)
	for i, r := range runes {
		runes[i] = unicode.ToLower(r)
	}
	return runes
}

// bestTermMatch tries each place term's first rune occurs in text, matching
// the rest greedily from there, and keeps the best-scoring match.
func bestTermMatch(term, text []rune) (best int, bestPos []int, found bool) {
	for start, r := range text {
		if r != term[0] {
			continue
		}
		pos := []int{start}
		for _, t := range term[1:] {
			i := slices.Index(text[pos[len(pos)-1]+1:], t)
			if i < 0 {
				break
			}
			pos = append(pos, pos[len(pos)-1]+1+i)
		}
		if len(pos) < len(term) {
			break // later starts only have less text to match in
		}
		if s := termScore(text, pos); !found || s > best {
			best, bestPos, found = s, pos, true
		}
	}
	return best, bestPos, found
}

func termScore(text []rune, pos []int) int {
	score := 0
	for i, p := range pos {
		score += scoreMatch
		if p == 0 || isWordSep(text[p-1]) {
			score += scoreBoundary
		}
		if i > 0 {
			if gap := p - pos[i-1] - 1; gap == 0 {
				score += scoreConsecutive
			} else {
				score -= min(gap, maxGapPenalty)
			}
		}
	}
	return score
}

func isWordSep(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("/-_.~", r)
}

// highlightRunes renders text with the runes at pos (ascending) in
// ui.StyleFilterMatch.
func highlightRunes(text string, pos []int) string {
	if len(pos) == 0 {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	for i := 0; i < len(runes); {
		j := i
		matched := len(pos) > 0 && pos[0] == i
		for j < len(runes) && (len(pos) > 0 && pos[0] == j) == matched {
			if matched {
				pos = pos[1:]
			}
			j++
		}
		if matched {
			b.WriteString(ui.StyleFilterMatch.Render(string(runes[i:j])))
		} else {
			b.WriteString(string(runes[i:j]))
		}
		i = j
	}
	return b.String()
}
