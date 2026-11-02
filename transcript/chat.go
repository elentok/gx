package transcript

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Roles and kinds of a ChatMessage.
const (
	ChatRoleAgent  = "agent"
	ChatRolePerson = "person"

	ChatKindText    = "text"
	ChatKindPrompt  = "prompt"
	ChatKindCommand = "command"
	// ChatKindOutput is the stdout of a built-in command. It has the agent
	// role so the person-turn logic (rounds, grilling counts) ignores it, but
	// it is not an agent reply: consumers that read agent text match
	// ChatKindText.
	ChatKindOutput = "output"
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
	// Calls is one short line per tool call of a tools entry: the tool and its command or target.
	Calls []string
	At    time.Time
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
	// commandOutputRE matches the whole stdout line of a built-in command.
	commandOutputRE = regexp.MustCompile(`(?s)^<local-command-stdout>(.*?)</local-command-stdout>$`)
)

// MaxCommandOutputBytes caps one output entry; a cut entry ends with
// commandOutputCutMarker.
const (
	MaxCommandOutputBytes  = 8 * 1024
	commandOutputCutMarker = "[output cut]"
)

// compactedOutputPrefix opens the line /compact writes; the compacted entry
// already marks it.
const compactedOutputPrefix = "Compacted"

// commandOutput turns a command's stdout into an output entry, or drops it when
// it is empty or the /compact line. ANSI codes are stripped and the size capped.
func commandOutput(msg ChatMessage, stdout string) (ChatMessage, bool) {
	text := strings.TrimSpace(ansi.Strip(stdout))
	if text == "" || strings.HasPrefix(text, compactedOutputPrefix) {
		return ChatMessage{}, false
	}
	if len(text) > MaxCommandOutputBytes {
		text = strings.TrimRight(truncateText(text, MaxCommandOutputBytes), "…") + "\n" + commandOutputCutMarker
	}
	msg.Role, msg.Kind, msg.Text = ChatRoleAgent, ChatKindOutput, text
	return msg, true
}

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
	msgs, err = ReadChatFrom(f)
	if err != nil {
		return nil, false, err
	}
	return msgs, true, nil
}

// ReadChatFrom is ReadChat over an open transcript, for callers that hold the
// bytes in memory.
func ReadChatFrom(r io.Reader) (msgs []ChatMessage, err error) {
	scanner := bufio.NewScanner(r)
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
		for _, call := range line.toolCalls() {
			if run == nil {
				run = &ChatMessage{ID: line.UUID, Role: ChatRoleAgent, Kind: ChatKindTools, At: line.at()}
			}
			run.ToolCount++
			run.LastTool = call.Name
			run.Calls = append(run.Calls, call.Summary())
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return msgs, nil
}

func (l chatLine) at() time.Time {
	at, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	return at
}

// toolCalls lists the tool_use blocks an agent line holds, in order.
func (l chatLine) toolCalls() []contentBlock {
	if l.IsSidechain || l.Type != "assistant" {
		return nil
	}
	var calls []contentBlock
	for _, b := range decodeContentBlocks(l.Message.Content) {
		if b.Type == "tool_use" {
			calls = append(calls, b)
		}
	}
	return calls
}

// Summary is the call as one line: the tool name and its command or target.
func (b contentBlock) Summary() string {
	var in map[string]any
	_ = json.Unmarshal(b.Input, &in)
	return SummarizeCall(b.Name, in)
}

// SummarizeCall renders a tool name and its arguments as "Name: command-or-target".
func SummarizeCall(name string, in map[string]any) string {
	for _, k := range []string{"command", "cmd", "file_path", "path", "pattern", "url", "query", "description"} {
		switch v := in[k].(type) {
		case string:
			if v = strings.TrimSpace(v); v != "" {
				return name + ": " + v
			}
		case []any:
			// Codex shell commands are argv; the script is the last element.
			if n := len(v); n > 0 {
				if s, ok := v[n-1].(string); ok && strings.TrimSpace(s) != "" {
					return name + ": " + strings.TrimSpace(s)
				}
			}
		}
	}
	return name
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
	if out := commandOutputRE.FindStringSubmatch(text); out != nil {
		return commandOutput(msg, out[1])
	}
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
