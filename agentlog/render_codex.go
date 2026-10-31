package agentlog

import (
	"encoding/json"
	"strings"

	"github.com/elentok/gx/codexsession"
)

type codexLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Name    string `json:"name"`
		Input   string `json:"input"`
		Args    string `json:"arguments"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"payload"`
}

// RenderCodex turns one Codex rollout line into display lines, in Render's
// style. Reasoning, developer and injected lines, and unknown shapes, render
// to nil.
func RenderCodex(raw []byte) []string {
	var l codexLine
	if err := json.Unmarshal(raw, &l); err != nil || l.Type != "response_item" {
		return nil
	}
	p := l.Payload
	switch p.Type {
	case "message":
		if p.Role != "assistant" && p.Role != "user" {
			return nil
		}
		var out []string
		for _, c := range p.Content {
			t := strings.TrimSpace(c.Text)
			switch {
			case t == "":
			case p.Role == "assistant":
				out = append(out, tagged("text", t)...)
			case !codexsession.IsInjected(t):
				out = append(out, tagged("prompt", t)...)
			}
		}
		return out
	case "function_call":
		return []string{"[tool] " + p.Name + ": " + truncate(oneLine(p.Args))}
	case "custom_tool_call":
		return []string{"[tool] " + p.Name + ": " + truncate(oneLine(p.Input))}
	}
	return nil
}
