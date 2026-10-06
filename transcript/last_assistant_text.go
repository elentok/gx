package transcript

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxAssistantTextBytes caps what LastAssistantText returns, so a caller that
// stores or logs the text never carries a whole long answer.
const MaxAssistantTextBytes = 2048

// LastAssistantText returns the last non-sidechain assistant text block in the
// transcript at path, truncated to MaxAssistantTextBytes (on a rune boundary,
// with a trailing "…" when cut). It is the only reader here that keeps message
// content, so it is opt-in: nothing on the polling path calls it. ok is false
// if the file doesn't exist yet or holds no assistant text. Like
// LastAssistantUsage it scans from the tail, so it reuses parseLine/tailScan
// and the content-block decoding of ReadUnexecutedToolCall instead of a
// second parser.
func LastAssistantText(path string) (text string, ok bool, err error) {
	err = tailScan(path, func(lines []string) bool {
		for _, raw := range slices.Backward(lines) {
			line, parsed := parseContentLine(raw)
			if !parsed || line.Type != "assistant" || line.IsSidechain {
				continue
			}
			blocks := line.contentBlocks()
			for _, b := range slices.Backward(blocks) {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					text, ok = truncateText(strings.TrimSpace(b.Text), MaxAssistantTextBytes), true
					return true
				}
			}
		}
		return false
	})
	if err != nil {
		return "", false, err
	}
	return text, ok, nil
}

func parseContentLine(raw string) (unexecutedToolCallLine, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return unexecutedToolCallLine{}, false
	}
	var line unexecutedToolCallLine
	if json.Unmarshal([]byte(raw), &line) != nil {
		return unexecutedToolCallLine{}, false
	}
	return line, true
}

func truncateText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
