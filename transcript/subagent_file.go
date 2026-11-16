package transcript

import (
	"encoding/json"
	"os"
	"slices"
)

// SubagentDetails is what a subagent's own transcript shows.
type SubagentDetails struct {
	Model     string
	Tokens    int
	ToolCalls int
	// CurrentCall is the newest tool call with no result yet; empty when none.
	CurrentCall string
}

type subagentLine struct {
	Type    string `json:"type"`
	Message struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// subagentTokens is one API message's share of the total.
func subagentTokens(inputTokens, outputTokens int) int { return inputTokens + outputTokens }

// ReadSubagentFile reads a subagents/agent-<id>.jsonl. Every line of it is
// marked sidechain, so unlike the parent readers it keeps them. A missing file
// has no details.
func ReadSubagentFile(path string) (SubagentDetails, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SubagentDetails{}, nil
		}
		return SubagentDetails{}, err
	}
	defer f.Close()

	type open struct{ id, label string }
	var out SubagentDetails
	var calls []open
	seenMessages, seenCalls := map[string]bool{}, map[string]bool{}
	err = ForEachLine(f, func(b []byte) {
		var l subagentLine
		if json.Unmarshal(b, &l) != nil {
			return
		}
		if l.Type == "assistant" {
			if l.Message.Model != "" {
				out.Model = l.Message.Model
			}
			if id := l.Message.ID; id != "" && !seenMessages[id] {
				seenMessages[id] = true
				out.Tokens += subagentTokens(l.Message.Usage.Input, l.Message.Usage.Output)
			}
		}
		for _, blk := range decodeContentBlocks(l.Message.Content) {
			switch {
			case l.Type == "assistant" && blk.Type == "tool_use" && blk.ID != "":
				if seenCalls[blk.ID] {
					continue
				}
				seenCalls[blk.ID] = true
				out.ToolCalls++
				var in map[string]any
				_ = json.Unmarshal(blk.Input, &in)
				calls = append(calls, open{blk.ID, NewInFlightTool(blk.Name, in).Label})
			case l.Type == "user" && blk.Type == "tool_result":
				if i := slices.IndexFunc(calls, func(c open) bool { return c.id == blk.ToolUseID }); i >= 0 {
					calls = slices.Delete(calls, i, i+1)
				}
			}
		}
	})
	if err != nil {
		return SubagentDetails{}, err
	}
	if n := len(calls); n > 0 {
		out.CurrentCall = calls[n-1].label
	}
	return out, nil
}
