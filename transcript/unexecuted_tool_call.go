package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// unexecutedToolCallShapeRE matches a text block that reads like a bare
// function-call literal rather than a real answer: an identifier immediately
// followed by "(" and an opening "{" (e.g. "Agent({", "Bash({"). Anchored at
// the start of the (trimmed) text, since a genuine answer that happens to
// mention a call shape mid-sentence isn't the failure this detects.
var unexecutedToolCallShapeRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\(\{`)

// unexecutedToolCallLine is the subset of a transcript JSONL line
// ReadUnexecutedToolCall reads: enough of type/isSidechain/message.content to
// walk turn boundaries and classify each assistant content block.
type unexecutedToolCallLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// contentBlock is one entry of a message.content array — either a text run,
// a tool_use call, or (on a "user" line) a tool_result feeding a prior call's
// output back in. Only Type/Text are read here; ReadUnexecutedToolCall never
// needs a tool_use's input or a tool_result's payload, only that the block
// exists.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// isRealUserTurn reports whether l is the "user" line that opens a turn: a
// real user prompt, whose message.content is a plain string. A tool_result
// is also written as a "user" line, but its content is always a content-block
// array, never a bare string — that's the distinction this ticket's turn
// boundary rests on. Only such lines end the *previous* turn and start the
// next one; a tool_result "user" line is interior to the turn that produced
// the tool_use it answers.
func (l unexecutedToolCallLine) isRealUserTurn() bool {
	if l.Type != "user" || l.IsSidechain {
		return false
	}
	var s string
	return json.Unmarshal(l.Message.Content, &s) == nil
}

// contentBlocks decodes l.Message.Content into its content-block list.
// message.content on an assistant line is always a block array in practice,
// but a plain string is tolerated (decoded as a single text block) rather
// than dropped, matching decodeMessageText's leniency elsewhere in this
// codebase.
func (l unexecutedToolCallLine) contentBlocks() []contentBlock {
	if len(l.Message.Content) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(l.Message.Content, &s); err == nil {
		return []contentBlock{{Type: "text", Text: s}}
	}
	var blocks []contentBlock
	if err := json.Unmarshal(l.Message.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// ReadUnexecutedToolCall reads the transcript at path and reports whether its
// last turn — the lines from (and excluding) the last real-user-turn line
// (see isRealUserTurn) to EOF, or the whole file if no such line exists —
// ended with a text content block shaped like an unexecuted tool call
// (unexecutedToolCallShapeRE), with no tool_use content block anywhere in
// that same turn. Sidechain lines are excluded from the turn entirely, the
// same way ReadBackgroundTasks excludes them: a subagent's content is never
// part of the parent iteration's turn.
//
// A missing/unreadable file, or a transcript with no non-sidechain assistant
// line at all, reports false with a nil error rather than erroring — mirroring
// ReadBackgroundTasks' BackgroundTaskUnsupported/BackgroundTaskUnreadable
// handling, since callers here only ever need a plain match/no-match signal.
func ReadUnexecutedToolCall(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var turnLines []unexecutedToolCallLine
	totalNonBlank := 0
	parsedOK := 0

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		totalNonBlank++

		var entry unexecutedToolCallLine
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		parsedOK++

		if entry.IsSidechain {
			continue
		}
		if entry.isRealUserTurn() {
			turnLines = turnLines[:0]
			continue
		}
		turnLines = append(turnLines, entry)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return false, scanErr
	}
	if totalNonBlank > 0 && parsedOK == 0 {
		return false, nil
	}

	var lastAssistantBlocks []contentBlock
	sawAssistant := false
	for _, l := range turnLines {
		if l.Type != "assistant" {
			continue
		}
		sawAssistant = true
		blocks := l.contentBlocks()
		for _, b := range blocks {
			if b.Type == "tool_use" {
				return false, nil
			}
		}
		lastAssistantBlocks = blocks
	}
	if !sawAssistant || len(lastAssistantBlocks) == 0 {
		return false, nil
	}

	last := lastAssistantBlocks[len(lastAssistantBlocks)-1]
	if last.Type != "text" {
		return false, nil
	}
	return unexecutedToolCallShapeRE.MatchString(strings.TrimSpace(last.Text)), nil
}
