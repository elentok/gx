package codexsession

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
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

// RolloutPathIn returns the plain rollout file of sessionID under the Codex
// home. A live rollout in sessions wins over a plain one in archived_sessions.
func RolloutPathIn(home, sessionID string) (path string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	if path, ok, err = findRollout(filepath.Join(home, "sessions"), sessionID, ".jsonl"); ok || err != nil {
		return path, ok, err
	}
	return findRollout(filepath.Join(home, "archived_sessions"), sessionID, ".jsonl")
}

// ArchivedRolloutIn returns the zstd-compressed rollout of sessionID: Codex
// compresses old rollouts to .jsonl.zst. It is only the file's path; nothing is
// decoded here.
func ArchivedRolloutIn(home, sessionID string) (path string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	if path, ok, err = findRollout(filepath.Join(home, "archived_sessions"), sessionID, ".jsonl.zst"); ok || err != nil {
		return path, ok, err
	}
	return findRollout(filepath.Join(home, "sessions"), sessionID, ".jsonl.zst")
}

func findRollout(dir, sessionID, suffix string) (path string, ok bool, err error) {
	err = filepath.WalkDir(dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) || !strings.Contains(entry.Name(), sessionID) {
			return nil
		}
		path, ok = p, true
		return fs.SkipAll
	})
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	return path, ok, err
}

type chatRolloutLine struct {
	Timestamp string `json:"timestamp"`
	Ordinal   *int   `json:"ordinal"`
	Type      string `json:"type"`
	Payload   struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Role      string `json:"role"`
		Name      string `json:"name"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
		Input     string `json:"input"`
		// Output is a JSON string in the real lines.
		Output     json.RawMessage `json:"output"`
		CLIVersion string          `json:"cli_version"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"payload"`
}

// Prefixes of the lines Codex injects as user messages.
var injectedPrefixes = []string{"# AGENTS.md instructions", "<environment_context>", "<turn_aborted>"}

const gxCommandPrefix = "$gx-"

// imageOpenRE matches the wrapper text that opens a pasted image and captures
// its source path.
var imageOpenRE = regexp.MustCompile(`^<image\b[^>]*\bpath="([^"]*)"`)

// ReadChat turns a rollout into the visible messages: agent text, the person's
// prompts, folded tool runs and compactions. Reasoning, developer and injected
// lines stay hidden and unknown lines are skipped. A message's ID is its
// line's ordinal ("o20"), or "l<line>" when the line has none. imagesDir is the
// session's images folder; "" means none.
func ReadChat(r io.Reader, imagesDir string) (Chat, error) {
	return readChat(r, imagesDir, nil)
}

// ReadChatFromOrdinal reads a Codex child's rollout as the child's own
// conversation. A child rollout starts with a copy of the parent's history, so
// it skips every line before the first line whose ordinal is at least
// startOrdinal (the child's subagent_history_start_ordinal). A line without an
// ordinal is skipped too until that first line is reached, and kept after it.
// The session_meta line is always read, so Version is set.
func ReadChatFromOrdinal(r io.Reader, imagesDir string, startOrdinal int) (Chat, error) {
	return readChat(r, imagesDir, &startOrdinal)
}

func readChat(r io.Reader, imagesDir string, startOrdinal *int) (Chat, error) {
	var chat Chat
	reached := startOrdinal == nil
	var run *transcript.ChatMessage
	flush := func() {
		if run != nil {
			chat.Messages = append(chat.Messages, *run)
			run = nil
		}
	}
	n := 0
	err := transcript.ForEachLine(r, func(raw []byte) {
		n++
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil {
			return
		}
		id := fmt.Sprintf("l%d", n)
		if line.Ordinal != nil {
			id = fmt.Sprintf("o%d", *line.Ordinal)
			if !reached && *line.Ordinal >= *startOrdinal {
				reached = true
			}
		}
		if !reached && line.Type != "session_meta" {
			return
		}
		at, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
		p := line.Payload
		switch {
		case line.Type == "session_meta":
			chat.Version = p.CLIVersion
		case line.Type == "event_msg" && p.Type == "task_complete":
			chat.Turns++
		case line.Type == "event_msg" && p.Type == "turn_aborted":
			if p.Reason == "interrupted" {
				flush()
				chat.Messages = append(chat.Messages, transcript.ChatMessage{ID: id, Kind: transcript.ChatKindInterrupted, At: at})
			}
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
			args := map[string]any{}
			if json.Unmarshal([]byte(p.Arguments), &args) != nil {
				// A custom tool call's input is free text, such as a patch.
				args = map[string]any{"command": firstLine(p.Input)}
			}
			// A child's own spawn_agent calls get no link: no Codex grandchildren.
			if p.Name == "spawn_agent" && p.CallID != "" && startOrdinal == nil {
				desc, _ := args["task_name"].(string)
				run.SubagentCalls = append(run.SubagentCalls, transcript.SubagentCall{Index: len(run.Calls), CallID: p.CallID, Description: desc})
			}
			run.Calls = append(run.Calls, transcript.SummarizeCall(p.Name, args))
		case p.Type == "message" && (p.Role == "assistant" || p.Role == "user"):
			var raw []string
			var images []transcript.ChatImage
			inImage := false
			for _, c := range p.Content {
				if p.Role == "user" {
					if m := imageOpenRE.FindStringSubmatch(c.Text); m != nil {
						images = append(images, transcript.SavedImage(m[1], imagesDir))
						inImage = true
						continue
					}
					if strings.TrimSpace(c.Text) == "</image>" {
						inImage = false
						continue
					}
					if c.Type == "input_image" {
						if !inImage {
							images = append(images, transcript.ChatImage{Placeholder: true})
						}
						continue
					}
				}
				raw = append(raw, c.Text)
			}
			// A prompt carries images exactly when the loop collected one.
			var texts []string
			for _, t := range raw {
				if len(images) > 0 {
					t = transcript.StripImageChips(t)
				}
				if t = strings.TrimSpace(t); t != "" && (p.Role == "assistant" || !IsInjected(t)) {
					texts = append(texts, t)
				}
			}
			if len(texts) == 0 && len(images) == 0 {
				return
			}
			flush()
			msg := transcript.ChatMessage{ID: id, Text: strings.Join(texts, "\n\n"), Images: images, At: at}
			switch {
			case p.Role == "assistant":
				msg.Role, msg.Kind = transcript.ChatRoleAgent, transcript.ChatKindText
			case strings.HasPrefix(msg.Text, gxCommandPrefix), strings.HasPrefix(msg.Text, "/"):
				msg.Role, msg.Kind = transcript.ChatRolePerson, transcript.ChatKindCommand
			default:
				msg.Role, msg.Kind = transcript.ChatRolePerson, transcript.ChatKindPrompt
			}
			chat.Messages = append(chat.Messages, msg)
		}
	})
	if err != nil {
		return Chat{}, err
	}
	flush()
	return chat, nil
}

// IsInjected reports a user message Codex injected (AGENTS.md, environment),
// which no reader shows.
func IsInjected(text string) bool {
	for _, p := range injectedPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
