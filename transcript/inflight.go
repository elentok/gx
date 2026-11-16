package transcript

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
)

// In-flight tool kinds, as the Working row shows them.
const (
	InFlightShell  = "shell"
	InFlightAgent  = "agent"
	InFlightSearch = "search"
	InFlightOther  = "other"
)

// inFlightLabelCap is the longest label, ellipsis included.
const inFlightLabelCap = 60

// InFlightTool is a tool call the agent is running right now.
type InFlightTool struct {
	Kind  string
	Label string
}

// NewInFlightTool maps a tool name and its arguments to the kind and capped
// label the Working row shows. An unknown tool is "other", labelled by name.
func NewInFlightTool(name string, in map[string]any) InFlightTool {
	arg := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := in[k].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
		return ""
	}
	kind, value := InFlightOther, ""
	switch name {
	case "Bash":
		kind, value = InFlightShell, arg("command")
	case "exec_command":
		kind, value = InFlightShell, arg("cmd")
	case "Agent", "Task":
		kind, value = InFlightAgent, arg("description", "subagent_type")
	case "Grep", "Glob":
		kind, value = InFlightSearch, arg("pattern")
	case "WebSearch", "web_search":
		kind, value = InFlightSearch, arg("query")
	case "WebFetch":
		kind, value = InFlightSearch, arg("url")
	}
	if value == "" {
		value = name
	}
	return InFlightTool{Kind: kind, Label: capInFlightLabel(value)}
}

// capInFlightLabel keeps the first line, cut to the cap with an ellipsis.
func capInFlightLabel(s string) string {
	s = strings.TrimSpace(s)
	line, _, _ := strings.Cut(s, "\n")
	line = strings.TrimSpace(line)
	r := []rune(line)
	if len(r) > inFlightLabelCap {
		return string(r[:inFlightLabelCap]) + "…"
	}
	return line
}

// ReadInFlightTools lists the tool_use calls of the transcript's current turn
// that have no tool_result yet, newest first. The turn starts after the last
// real user line; sidechain lines are a sub-agent's and are skipped. A missing
// file has nothing in flight.
func ReadInFlightTools(path string) ([]InFlightTool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var t inFlightTracker
	err = ForEachLine(f, func(lineBytes []byte) {
		raw := strings.TrimSpace(string(lineBytes))
		if raw == "" {
			return
		}
		var line unexecutedToolCallLine
		if json.Unmarshal([]byte(raw), &line) != nil || line.IsSidechain {
			return
		}
		t.feed(line.Type, line.Message.Content)
	})
	if err != nil {
		return nil, err
	}
	return t.result(), nil
}

// inFlightTracker follows the open tool calls of the current turn over the
// non-sidechain lines of a transcript. ReadTaskRecords shares it so one scan
// yields both.
type inFlightTracker struct {
	calls []openCall
}

type openCall struct {
	id   string
	tool InFlightTool
}

// feed takes one non-sidechain line's type and message.content.
func (t *inFlightTracker) feed(lineType string, content json.RawMessage) {
	if lineType == "user" {
		var s string
		if json.Unmarshal(content, &s) == nil {
			t.calls = t.calls[:0]
			return
		}
	}
	for _, b := range decodeContentBlocks(content) {
		switch {
		case lineType == "assistant" && b.Type == "tool_use" && b.ID != "":
			var in map[string]any
			_ = json.Unmarshal(b.Input, &in)
			t.calls = append(t.calls, openCall{b.ID, NewInFlightTool(b.Name, in)})
		case lineType == "user" && b.Type == "tool_result":
			if i := slices.IndexFunc(t.calls, func(c openCall) bool { return c.id == b.ToolUseID }); i >= 0 {
				t.calls = slices.Delete(t.calls, i, i+1)
			}
		}
	}
}

// result lists the open calls newest first.
func (t *inFlightTracker) result() []InFlightTool {
	out := make([]InFlightTool, 0, len(t.calls))
	for _, c := range slices.Backward(t.calls) {
		out = append(out, c.tool)
	}
	return out
}
