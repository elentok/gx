package transcript

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxAssistantTextBytes is the cap for a caller that stores or logs the text,
// so it never carries a whole long answer.
const MaxAssistantTextBytes = 2048

// NoTextCap makes LastAssistantText return the whole block, for a caller that
// parses the text and would misread a cut one.
const NoTextCap = 0

// LastAssistantText returns the last non-sidechain assistant text block in the
// transcript at path, truncated to maxBytes (on a rune boundary, with a
// trailing "…" when cut; NoTextCap keeps it whole). It is the only reader here that keeps message
// content, so it is opt-in: nothing on the polling path calls it. ok is false
// if the file doesn't exist yet or holds no assistant text. Like
// LastAssistantUsage it scans from the tail, so it reuses parseLine/tailScan
// and the content-block decoding of ReadUnexecutedToolCall instead of a
// second parser.
func LastAssistantText(path string, maxBytes int) (text string, ok bool, err error) {
	err = tailScan(path, func(lines []string) bool {
		for _, raw := range slices.Backward(lines) {
			line, parsed := parseContentLine(raw)
			if !parsed || line.Type != "assistant" || line.IsSidechain {
				continue
			}
			blocks := line.contentBlocks()
			for _, b := range slices.Backward(blocks) {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					text, ok = strings.TrimSpace(b.Text), true
					if maxBytes != NoTextCap {
						text = truncateText(text, maxBytes)
					}
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
