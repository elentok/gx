package transcript

import "slices"

// TurnEnded reports whether the transcript at path holds the end of its last
// turn: a turn_duration line after the last user or assistant line. Claude
// writes that line last, and only after it reports itself idle, so a reader
// that waits for it sees the turn's final message. A missing file has not
// ended.
func TurnEnded(path string) (bool, error) {
	ended := false
	err := tailScan(path, func(lines []string) bool {
		for _, raw := range slices.Backward(lines) {
			line, ok := parseLine(raw)
			if !ok {
				continue
			}
			switch {
			case line.Type == "system" && line.Subtype == "turn_duration":
				ended = true
				return true
			case line.Type == "user" || line.Type == "assistant":
				return true
			}
		}
		return false
	})
	return ended, err
}
