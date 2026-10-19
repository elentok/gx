package claudehistory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	toolResultLineLimit  = 50
	errorResultLineLimit = 100
)

// ExportMarkdown reads the transcript at path and renders it as readable Markdown.
// Noise records (system, isMeta, command wrappers) are stripped.
// Sidechain turns are included and marked.
func ExportMarkdown(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var parts []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var rec exportLine
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}

		switch rec.Type {
		case "user", "assistant":
		default:
			continue
		}

		if rec.IsMeta {
			continue
		}

		if len(rec.RawMessage) == 0 {
			continue
		}

		var msg exportMessage
		if err := json.Unmarshal(rec.RawMessage, &msg); err != nil {
			continue
		}

		rendered := renderTurn(rec, msg)
		if rendered == "" {
			continue
		}
		parts = append(parts, rendered)
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(parts) == 0 {
		return "", nil
	}

	return strings.Join(parts, "\n\n---\n\n") + "\n", nil
}

type exportLine struct {
	Type        string          `json:"type"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	RawMessage  json.RawMessage `json:"message"`
}

type exportMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type exportBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func renderTurn(rec exportLine, msg exportMessage) string {
	role := msg.Role
	if role == "" {
		role = rec.Type
	}

	var heading string
	switch role {
	case "user":
		if rec.IsSidechain {
			heading = "## User (subagent)"
		} else {
			heading = "## User"
		}
	case "assistant":
		if rec.IsSidechain {
			heading = "## Assistant (subagent)"
		} else {
			heading = "## Assistant"
		}
	default:
		return ""
	}

	var rendered []string

	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		text := cleanUserText(s)
		if text == "" {
			return ""
		}
		rendered = append(rendered, text)
	} else {
		var blocks []exportBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			return ""
		}
		for _, b := range blocks {
			r := renderBlock(b)
			if r != "" {
				rendered = append(rendered, r)
			}
		}
	}

	if len(rendered) == 0 {
		return ""
	}

	return heading + "\n\n" + strings.Join(rendered, "\n\n")
}

func renderBlock(b exportBlock) string {
	switch b.Type {
	case "text":
		return strings.TrimSpace(b.Text)

	case "thinking":
		if b.Thinking == "" {
			return ""
		}
		return "<details>\n<summary>Thinking</summary>\n\n" + strings.TrimSpace(b.Thinking) + "\n</details>"

	case "tool_use":
		return renderToolUse(b)

	case "tool_result":
		return renderToolResult(b)

	default:
		return ""
	}
}

func renderToolUse(b exportBlock) string {
	inputJSON := "{}"
	if len(b.Input) > 0 {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		var v any
		if err := json.Unmarshal(b.Input, &v); err == nil {
			_ = enc.Encode(v)
			inputJSON = strings.TrimSpace(buf.String())
		} else {
			inputJSON = string(b.Input)
		}
	}
	return fmt.Sprintf("**%s**\n```json\n%s\n```", b.Name, inputJSON)
}

func renderToolResult(b exportBlock) string {
	text := extractToolResultText(b.Content)

	prefix := "**Result:**"
	if b.IsError {
		prefix = "**Result (error):**"
	}

	limit := toolResultLineLimit
	if b.IsError {
		limit = errorResultLineLimit
	}

	return fmt.Sprintf("%s\n```\n%s\n```", prefix, headAndTail(text, limit))
}

// headAndTail keeps the first and last limit/2 lines of text and replaces the
// middle with an omitted-lines marker. The end of a result (an error, a test
// summary) is usually what matters, so a head-only cut is not enough.
func headAndTail(text string, limit int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= limit {
		return text
	}
	keep := limit / 2
	omitted := len(lines) - 2*keep
	out := append([]string{}, lines[:keep]...)
	out = append(out, fmt.Sprintf("... (%d lines omitted)", omitted))
	out = append(out, lines[len(lines)-keep:]...)
	return strings.Join(out, "\n")
}

func extractToolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
