package preview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui"
)

// highlightPreviewLine wraps query's first match on an already
// glamour-rendered (ANSI-styled) line in the search-highlight style. It
// mirrors search.Highlight's byte-offset matching, but walks ANSI sequences
// before rebuilding the line so it never splits an escape sequence. ANSI
// styling inside the matched run is deliberately replaced by the overlay;
// preserving it would let its resets cancel the search highlight.
func highlightPreviewLine(line, query string, current bool) string {
	plain := ansi.Strip(line)
	lower := strings.ToLower(plain)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return line
	}
	end := min(idx+len(query), len(plain))

	style := ui.StyleSearchResult
	if current {
		style = ui.StyleActiveSearchResult
	}

	var prefix, matched, suffix strings.Builder
	state := ansi.NormalState
	plainOffset := 0
	matchStarted := false
	matchEnded := false
	for rest := line; len(rest) > 0; {
		seq, _, n, nextState := ansi.DecodeSequence(rest, state, nil)
		if n == 0 {
			// DecodeSequence only returns zero for malformed input. Copy its
			// remaining bytes unchanged rather than risking an infinite loop.
			suffix.WriteString(rest)
			break
		}
		state = nextState
		rest = rest[n:]

		plainSeq := ansi.Strip(seq)
		if plainSeq == "" {
			switch {
			case !matchStarted:
				prefix.WriteString(seq)
			case matchEnded:
				suffix.WriteString(seq)
			}
			continue
		}

		seqStart := plainOffset
		plainOffset += len(plainSeq)
		if seqStart < end && plainOffset > idx {
			matchStarted = true
			matched.WriteString(plainSeq)
			continue
		}
		if matchStarted {
			matchEnded = true
		}
		if plainOffset <= idx {
			prefix.WriteString(seq)
		} else {
			suffix.WriteString(seq)
		}
	}

	return prefix.String() + style.Render(matched.String()) + suffix.String()
}
