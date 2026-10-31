package codexsession

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

// Chat is what one read of a rollout yields. Turns counts the finished turns
// (task_complete lines) and Version is the rollout's cli_version.
type Chat struct {
	Messages []transcript.ChatMessage
	Turns    int
	Version  string
}

// Unreadable reports a rollout that has turns but no message gx could read:
// the line shapes changed under it.
func (c Chat) Unreadable() bool { return c.Turns > 0 && len(c.Messages) == 0 }

// RolloutPathIn returns the rollout file of sessionID under the Codex home.
func RolloutPathIn(home, sessionID string) (path string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	err = walkSessionFiles(home, sessionID, func(p string) error {
		path, ok = p, true
		return fs.SkipAll
	})
	if err != nil {
		return "", false, err
	}
	return path, ok, nil
}

type chatRolloutLine struct {
	Timestamp string `json:"timestamp"`
	Ordinal   *int   `json:"ordinal"`
	Type      string `json:"type"`
	Payload   struct {
		Type       string `json:"type"`
		Role       string `json:"role"`
		Name       string `json:"name"`
		CLIVersion string `json:"cli_version"`
		Content    []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"payload"`
}

// Prefixes of the lines Codex injects as user messages.
var injectedPrefixes = []string{"# AGENTS.md instructions", "<environment_context>"}

const gxCommandPrefix = "$gx-"

// ReadChat turns a rollout into the visible messages: agent text, the person's
// prompts, folded tool runs and compactions. Reasoning, developer and injected
// lines stay hidden and unknown lines are skipped. A message's ID is its
// line's ordinal ("o20"), or "l<line>" when the line has none.
func ReadChat(r io.Reader) (Chat, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var chat Chat
	var run *transcript.ChatMessage
	flush := func() {
		if run != nil {
			chat.Messages = append(chat.Messages, *run)
			run = nil
		}
	}
	for n := 1; scanner.Scan(); n++ {
		var line chatRolloutLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		id := fmt.Sprintf("l%d", n)
		if line.Ordinal != nil {
			id = fmt.Sprintf("o%d", *line.Ordinal)
		}
		at, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		p := line.Payload
		switch {
		case line.Type == "session_meta":
			chat.Version = p.CLIVersion
		case line.Type == "event_msg" && p.Type == "task_complete":
			chat.Turns++
		case line.Type == "compacted":
			flush()
			chat.Messages = append(chat.Messages, transcript.ChatMessage{ID: id, Kind: transcript.ChatKindCompacted, At: at})
		case line.Type != "response_item":
		case p.Type == "custom_tool_call" || p.Type == "function_call":
			if run == nil {
				run = &transcript.ChatMessage{ID: id, Role: transcript.ChatRoleAgent, Kind: transcript.ChatKindTools, At: at}
			}
			run.ToolCount++
			run.LastTool = p.Name
		case p.Type == "message" && (p.Role == "assistant" || p.Role == "user"):
			var texts []string
			for _, c := range p.Content {
				if t := strings.TrimSpace(c.Text); t != "" && (p.Role == "assistant" || !isInjected(t)) {
					texts = append(texts, t)
				}
			}
			if len(texts) == 0 {
				continue
			}
			flush()
			msg := transcript.ChatMessage{ID: id, Text: strings.Join(texts, "\n\n"), At: at}
			switch {
			case p.Role == "assistant":
				msg.Role, msg.Kind = transcript.ChatRoleAgent, transcript.ChatKindText
			case strings.HasPrefix(msg.Text, gxCommandPrefix):
				msg.Role, msg.Kind = transcript.ChatRolePerson, transcript.ChatKindCommand
			default:
				msg.Role, msg.Kind = transcript.ChatRolePerson, transcript.ChatKindPrompt
			}
			chat.Messages = append(chat.Messages, msg)
		}
	}
	if err := scanner.Err(); err != nil {
		return Chat{}, err
	}
	flush()
	return chat, nil
}

func isInjected(text string) bool {
	for _, p := range injectedPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}
