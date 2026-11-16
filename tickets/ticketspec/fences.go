package ticketspec

import (
	"iter"
	"strings"
)

// fenceLine is one line of text with its code-fence state.
type fenceLine struct {
	Text    string
	InFence bool
	// Info is the opening fence's info string (e.g. "text"); empty outside a fence.
	Info string
}

// fenceLines yields every line of text except the ``` delimiter lines
// themselves. A fence left open runs to the end of the text.
func fenceLines(text string) iter.Seq[fenceLine] {
	return func(yield func(fenceLine) bool) {
		inFence, info := false, ""
		for line := range strings.SplitSeq(text, "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
				info = ""
				if inFence {
					info = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				}
				continue
			}
			if !yield(fenceLine{Text: line, InFence: inFence, Info: info}) {
				return
			}
		}
	}
}
