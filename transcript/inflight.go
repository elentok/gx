package transcript

import (
	"bufio"
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
	case "web_search":
		kind, value = InFlightSearch, arg("query")
	case "Agent", "Task":
		kind, value = InFlightAgent, arg("description", "subagent_type")
	case "Grep", "Glob":
		kind, value = InFlightSearch, arg("pattern")
	case "WebSearch":
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

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	type open struct {
		id   string
		tool InFlightTool
	}
	var calls []open
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var line unexecutedToolCallLine
		if json.Unmarshal([]byte(raw), &line) != nil || line.IsSidechain {
			continue
		}
		if line.isRealUserTurn() {
			calls = calls[:0]
			continue
		}
		for _, b := range line.contentBlocks() {
			switch {
			case line.Type == "assistant" && b.Type == "tool_use" && b.ID != "":
				var in map[string]any
				_ = json.Unmarshal(b.Input, &in)
				calls = append(calls, open{b.ID, NewInFlightTool(b.Name, in)})
			case line.Type == "user" && b.Type == "tool_result":
				for i, c := range calls {
					if c.id == b.ToolUseID {
						calls = append(calls[:i], calls[i+1:]...)
						break
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]InFlightTool, 0, len(calls))
	for _, c := range slices.Backward(calls) {
		out = append(out, c.tool)
	}
	return out, nil
}
