package tickets

import "strings"

// ParseIterationBranch reports whether branch matches ralphloop's iteration
// naming and, if so, the epic and ticket identifier it encodes. Two
// generations of naming are recognized: the current flat
// "ralph-loop/{epic}-item-{id}" (ralphloop/labels.go's iterBranch) and the
// legacy nested "ralph-loop/{epic}/iter-{id}" this repo's own history still
// carries.
func ParseIterationBranch(branch string) (epic, ticketID string, ok bool) {
	rest, ok := strings.CutPrefix(branch, "ralph-loop/")
	if !ok {
		return "", "", false
	}

	if idx := strings.LastIndex(rest, "-item-"); idx != -1 {
		epic, ticketID = rest[:idx], rest[idx+len("-item-"):]
		if epic != "" && ticketID != "" {
			return epic, ticketID, true
		}
	}

	if epicName, tail, cutOK := strings.Cut(rest, "/"); cutOK {
		if id, hasPrefix := strings.CutPrefix(tail, "iter-"); hasPrefix && epicName != "" && id != "" {
			return epicName, id, true
		}
	}

	return "", "", false
}
