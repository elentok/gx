package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"
)

// Roles and kinds of a ChatMessage.
const (
	ChatRoleAgent  = "agent"
	ChatRolePerson = "person"

	ChatKindText    = "text"
	ChatKindPrompt  = "prompt"
	ChatKindCommand = "command"
	// ChatKindTools folds a run of tool calls; ChatKindCompacted marks a
	// compaction.
	ChatKindTools     = "tools"
	ChatKindCompacted = "compacted"
)

// ChatMessage is one visible entry of a conversation. ID is the transcript
// line's uuid. A tools entry carries ToolCount and LastTool instead of Text.
type ChatMessage struct {
	ID        string
	Role      string
	Kind      string
	Text      string
	ToolCount int
	LastTool  string
	At        time.Time
}

type chatLine struct {
	Type             string `json:"type"`
	Subtype          string `json:"subtype"`
	UUID             string `json:"uuid"`
	Timestamp        string `json:"timestamp"`
	IsSidechain      bool   `json:"isSidechain"`
	IsMeta           bool   `json:"isMeta"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

var (
	commandNameRE = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	commandArgsRE = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// localCommandPrefix opens the caveat and output lines Claude Code writes
// around a built-in slash command; the person never typed them.
const localCommandPrefix = "<local-command"

// taskNotificationPrefix opens the line Claude Code writes when a background
// task ends. It is not the person's reply, so a round it follows stays open.
const taskNotificationPrefix = "<task-notification>"

// ReadChat returns the transcript's visible messages in order: agent text,
// the person's real prompts and slash commands. Thinking, system lines, meta
// lines, attachments, compact summaries, sidechains and tool results stay
// hidden. Each run of tool calls between two visible messages folds into one
// tools entry, and a compaction becomes a compacted entry. ok is false if the
// file doesn't exist yet.
func ReadChat(path string) (msgs []ChatMessage, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var run *ChatMessage
	flush := func() {
		if run != nil {
			msgs = append(msgs, *run)
			run = nil
		}
	}
	for scanner.Scan() {
		var line chatLine
		if json.Unmarshal([]byte(strings.TrimSpace(scanner.Text())), &line) != nil {
			continue
		}
		if line.Type == "system" && line.Subtype == compactBoundarySubtype {
			flush()
			msgs = append(msgs, ChatMessage{ID: line.UUID, Kind: ChatKindCompacted, At: line.at()})
			continue
		}
		if msg, visible := line.message(); visible {
			flush()
			msgs = append(msgs, msg)
		}
		for _, name := range line.toolNames() {
			if run == nil {
				run = &ChatMessage{ID: line.UUID, Role: ChatRoleAgent, Kind: ChatKindTools, At: line.at()}
			}
			run.ToolCount++
			run.LastTool = name
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	flush()
	return msgs, true, nil
}

func (l chatLine) at() time.Time {
	at, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	return at
}

// toolNames lists the tools an agent line calls, in order.
func (l chatLine) toolNames() []string {
	if l.IsSidechain || l.Type != "assistant" {
		return nil
	}
	var names []string
	for _, b := range decodeContentBlocks(l.Message.Content) {
		if b.Type == "tool_use" {
			names = append(names, b.Name)
		}
	}
	return names
}

func (l chatLine) message() (ChatMessage, bool) {
	if l.IsSidechain || l.IsMeta || l.IsCompactSummary || (l.Type != "assistant" && l.Type != "user") {
		return ChatMessage{}, false
	}
	var texts []string
	for _, b := range decodeContentBlocks(l.Message.Content) {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			texts = append(texts, strings.TrimSpace(b.Text))
		}
	}
	if len(texts) == 0 {
		return ChatMessage{}, false
	}
	text := strings.Join(texts, "\n\n")
	msg := ChatMessage{ID: l.UUID, Text: text, At: l.at()}
	if l.Type == "assistant" {
		msg.Role, msg.Kind = ChatRoleAgent, ChatKindText
		return msg, true
	}
	msg.Role = ChatRolePerson
	if strings.HasPrefix(text, localCommandPrefix) || strings.HasPrefix(text, taskNotificationPrefix) {
		return ChatMessage{}, false
	}
	if name := commandNameRE.FindStringSubmatch(text); name != nil {
		msg.Kind = ChatKindCommand
		msg.Text = strings.TrimSpace(name[1])
		if args := commandArgsRE.FindStringSubmatch(text); args != nil {
			msg.Text = strings.TrimSpace(msg.Text + " " + strings.TrimSpace(args[1]))
		}
		return msg, true
	}
	msg.Kind = ChatKindPrompt
	return msg, true
}
