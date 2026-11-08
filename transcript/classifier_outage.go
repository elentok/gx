package transcript

import (
	"encoding/json"
	"slices"
	"strings"
)

// classifierNoVerdict is the text of the tool error claude shows when the
// auto-mode classifier is down; every tool call then fails with it.
const classifierNoVerdict = "auto mode classifier gave no verdict"

type toolResultLine struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type toolResultBlock struct {
	Type    string          `json:"type"`
	IsError bool            `json:"is_error"`
	Content json.RawMessage `json:"content"`
}

// CountClassifierNoVerdict counts the consecutive "no verdict" tool-result
// errors at the end of the transcript; 0 when the tail is anything else. The
// assistant lines between failed calls do not break the run.
func CountClassifierNoVerdict(path string) (n int, err error) {
	err = tailScan(path, func(lines []string) bool {
		n = 0
		for _, raw := range slices.Backward(lines) {
			var line toolResultLine
			if json.Unmarshal([]byte(raw), &line) != nil || line.Type == "assistant" {
				continue
			}
			if line.Type != "user" {
				continue
			}
			var blocks []toolResultBlock
			if json.Unmarshal(line.Message.Content, &blocks) != nil || len(blocks) == 0 {
				return true
			}
			for _, b := range blocks {
				if b.Type != "tool_result" || !b.IsError || !strings.Contains(contentText(b.Content), classifierNoVerdict) {
					return true
				}
				n++
			}
		}
		return false
	})
	return n, err
}
