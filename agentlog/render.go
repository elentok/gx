// Package agentlog finds an agent's log and renders it as text. One renderer
// serves `gx server agents watch` and the TUI "Watch agent" modal, so every
// view looks the same. It reads both the headless runner's out.jsonl
// (stream-json events) and a claude transcript: their assistant and user
// lines share one shape.
package agentlog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxSummary caps a one-line summary of a tool call or tool result. The full
// payload stays one --json away.
const maxSummary = 160

type logLine struct {
	Type        string  `json:"type"`
	Subtype     string  `json:"subtype"`
	IsMeta      bool    `json:"isMeta"`
	IsSidechain bool    `json:"isSidechain"`
	Message     message `json:"message"`
	Result      string  `json:"result"`
	IsError     bool    `json:"is_error"`
	SessionID   string  `json:"session_id"`
}

type message struct {
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
	IsError  bool            `json:"is_error"`
}

// Render turns one JSONL line into display lines. Lines with nothing to show
// (stream deltas, rate-limit events, bookkeeping) render to nil.
func Render(raw []byte) []string {
	var l logLine
	if err := json.Unmarshal(raw, &l); err != nil || l.IsMeta || l.IsSidechain {
		return nil
	}
	switch l.Type {
	case "assistant":
		return renderBlocks(l.Message.Content, false)
	case "user":
		return renderBlocks(l.Message.Content, true)
	case "result":
		if l.IsError || strings.HasPrefix(l.Subtype, "error") {
			return tagged("result", "error: "+firstNonEmpty(l.Result, l.Subtype))
		}
		return tagged("result", l.Result)
	case "system":
		switch l.Subtype {
		case "init":
			return tagged("session", l.SessionID)
		case "compact_boundary":
			return []string{"[compact]"}
		}
	}
	return nil
}

func renderBlocks(content json.RawMessage, user bool) []string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		if user {
			return tagged("prompt", s)
		}
		return tagged("text", s)
	}
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if user {
				out = append(out, tagged("prompt", b.Text)...)
			} else {
				out = append(out, tagged("text", b.Text)...)
			}
		case "thinking":
			out = append(out, tagged("thinking", b.Thinking)...)
		case "tool_use":
			out = append(out, fmt.Sprintf("[tool] %s: %s", b.Name, toolSummary(b.Input)))
		case "tool_result":
			tag := "tool-result"
			if b.IsError {
				tag = "tool-error"
			}
			out = append(out, fmt.Sprintf("[%s] %s", tag, truncate(oneLine(resultText(b.Content)))))
		}
	}
	return out
}

// tagged prefixes the first line of text with [tag] and indents the rest, so
// multi-line text stays readable and still greps by tag.
func tagged(tag, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	lines[0] = "[" + tag + "] " + lines[0]
	for i := 1; i < len(lines); i++ {
		lines[i] = "  " + lines[i]
	}
	return lines
}

// toolSummary picks the input field that says what a tool call does, falling
// back to the compact input JSON for tools it doesn't know.
func toolSummary(input json.RawMessage) string {
	var fields map[string]any
	if json.Unmarshal(input, &fields) == nil {
		for _, k := range []string{"command", "file_path", "pattern", "url", "query", "description", "prompt"} {
			if v, ok := fields[k].(string); ok && v != "" {
				return truncate(oneLine(v))
			}
		}
	}
	return truncate(string(input))
}

// resultText reads a tool_result's content, a string or a list of blocks.
func resultText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string) string {
	r := []rune(s)
	if len(r) <= maxSummary {
		return s
	}
	return string(r[:maxSummary-1]) + "…"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
